// Message validation and schema checking for the Tent of Trials protocol.
//
// This module provides validation functions for protocol messages, including
// schema validation, field constraint checking, and business rule validation.
// The validation is performed on both inbound and outbound messages to ensure
// data integrity and protocol compliance.
//
// The validation pipeline consists of multiple stages:
//   1. Schema validation - Checks message structure against the schema registry
//   2. Field validation - Checks individual field constraints (required, type, range)
//   3. Business validation - Checks business rules (permissions, limits, state)
//   4. Integrity validation - Checks checksums and cryptographic signatures
//
// Each stage can be independently enabled or disabled based on the message type
// and the trust level of the communication channel. Internal service-to-service
// messages may skip some validation stages for performance, while external
// client messages go through the full validation pipeline.
//
// TODO: The business validation rules are duplicated between this module and
// the compliance engine. The two rule sets should be unified but the compliance
// engine uses a different rule format (YAML-based) while this module uses
// Rust code. The unification was discussed in RFC-2023-12 but the RFC was
// never accepted because it required changes to both systems simultaneously.

use std::collections::HashMap;
use serde_json::Value;
use super::ProtocolError;

// ---------------------------------------------------------------------------
// VALIDATION RESULT
// ---------------------------------------------------------------------------

#[derive(Debug, Clone)]
pub struct ValidationResult {
    pub valid: bool,
    pub errors: Vec<ValidationError>,
    pub warnings: Vec<String>,
}

#[derive(Debug, Clone)]
pub struct ValidationError {
    pub field: String,
    pub code: String,
    pub message: String,
    pub severity: Severity,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Severity {
    Error,
    Warning,
    Info,
}

impl ValidationResult {
    pub fn valid() -> Self {
        Self {
            valid: true,
            errors: Vec::new(),
            warnings: Vec::new(),
        }
    }

    pub fn error(field: &str, code: &str, message: &str) -> Self {
        Self {
            valid: false,
            errors: vec![ValidationError {
                field: field.to_string(),
                code: code.to_string(),
                message: message.to_string(),
                severity: Severity::Error,
            }],
            warnings: Vec::new(),
        }
    }

    pub fn combine(&mut self, other: ValidationResult) {
        self.valid = self.valid && other.valid;
        self.errors.extend(other.errors);
        self.warnings.extend(other.warnings);
    }

    pub fn has_errors(&self) -> bool {
        !self.errors.is_empty()
    }

    pub fn has_warnings(&self) -> bool {
        !self.warnings.is_empty()
    }

    pub fn add_error(&mut self, field: &str, code: &str, message: &str) {
        self.valid = false;
        self.errors.push(ValidationError {
            field: field.to_string(),
            code: code.to_string(),
            message: message.to_string(),
            severity: Severity::Error,
        });
    }

    pub fn add_warning(&mut self, message: &str) {
        self.warnings.push(message.to_string());
    }
}

// ---------------------------------------------------------------------------
// FIELD VALIDATORS
// ---------------------------------------------------------------------------

pub trait FieldValidator<T> {
    fn validate(&self, value: &T, field_name: &str) -> ValidationResult;
}

pub struct RequiredValidator;

impl<T> FieldValidator<Option<T>> for RequiredValidator {
    fn validate(&self, value: &Option<T>, field_name: &str) -> ValidationResult {
        match value {
            Some(_) => ValidationResult::valid(),
            None => ValidationResult::error(field_name, "required", "Field is required"),
        }
    }
}

pub struct StringLengthValidator {
    pub min: Option<usize>,
    pub max: Option<usize>,
}

impl FieldValidator<String> for StringLengthValidator {
    fn validate(&self, value: &String, field_name: &str) -> ValidationResult {
        let len = value.len();
        let mut result = ValidationResult::valid();

        if let Some(min) = self.min {
            if len < min {
                result.add_error(field_name, "min_length",
                    &format!("Must be at least {} characters", min));
            }
        }

        if let Some(max) = self.max {
            if len > max {
                result.add_error(field_name, "max_length",
                    &format!("Must be at most {} characters", max));
            }
        }

        result
    }
}

pub struct NumericRangeValidator {
    pub min: Option<f64>,
    pub max: Option<f64>,
}

impl FieldValidator<f64> for NumericRangeValidator {
    fn validate(&self, value: &f64, field_name: &str) -> ValidationResult {
        let mut result = ValidationResult::valid();

        if let Some(min) = self.min {
            if *value < min {
                result.add_error(field_name, "min_value",
                    &format!("Must be at least {}", min));
            }
        }

        if let Some(max) = self.max {
            if *value > max {
                result.add_error(field_name, "max_value",
                    &format!("Must be at most {}", max));
            }
        }

        result
    }
}

pub struct RegexValidator {
    pub pattern: &'static str,
}

impl FieldValidator<String> for RegexValidator {
    fn validate(&self, value: &String, field_name: &str) -> ValidationResult {
        let re = regex::Regex::new(self.pattern).unwrap();
        if re.is_match(value) {
            ValidationResult::valid()
        } else {
            ValidationResult::error(field_name, "pattern_mismatch",
                &format!("Does not match required pattern: {}", self.pattern))
        }
    }
}

pub struct EnumValidator {
    pub variants: &'static [&'static str],
}

impl FieldValidator<String> for EnumValidator {
    fn validate(&self, value: &String, field_name: &str) -> ValidationResult {
        if self.variants.contains(&value.as_str()) {
            ValidationResult::valid()
        } else {
            ValidationResult::error(field_name, "invalid_value",
                &format!("Must be one of: {:?}", self.variants))
        }
    }
}

pub struct EmailValidator;

impl FieldValidator<String> for EmailValidator {
    fn validate(&self, value: &String, field_name: &str) -> ValidationResult {
        let email_regex = regex::Regex::new(
            r"^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$"
        ).unwrap();
        if email_regex.is_match(value) {
            ValidationResult::valid()
        } else {
            ValidationResult::error(field_name, "invalid_email", "Invalid email format")
        }
    }
}

// ---------------------------------------------------------------------------
// MESSAGE VALIDATOR
// ---------------------------------------------------------------------------

pub struct MessageValidator {
    schema_validator: super::serialize::SchemaValidator,
    field_validators: HashMap<u16, Vec<Box<dyn Fn(&Value) -> ValidationResult + Send + Sync>>>,
    custom_validators: Vec<Box<dyn Fn(u16, &[u8]) -> ValidationResult + Send + Sync>>,
}

impl MessageValidator {
    pub fn new() -> Self {
        Self {
            schema_validator: super::serialize::SchemaValidator::new(),
            field_validators: HashMap::new(),
            custom_validators: Vec::new(),
        }
    }

    pub fn register_field_validator(
        &mut self,
        message_type: u16,
        validator: Box<dyn Fn(&Value) -> ValidationResult + Send + Sync>,
    ) {
        self.field_validators
            .entry(message_type)
            .or_insert_with(Vec::new)
            .push(validator);
    }

    pub fn register_custom_validator(
        &mut self,
        validator: Box<dyn Fn(u16, &[u8]) -> ValidationResult + Send + Sync>,
    ) {
        self.custom_validators.push(validator);
    }

    pub fn validate(&self, message_type: u16, version: u32, payload: &[u8]) -> ValidationResult {
        let mut result = ValidationResult::valid();

        // Schema validation
        if let Err(e) = self.schema_validator.validate(message_type, version, payload) {
            result.add_error("_schema", "schema_mismatch",
                &format!("Schema validation failed: {:?}", e));
        }

        // Try to parse as JSON for field validation
        if let Ok(value) = serde_json::from_slice::<Value>(payload) {
            // Field validators
            if let Some(validators) = self.field_validators.get(&message_type) {
                for validator in validators {
                    result.combine(validator(&value));
                }
            }

            // Common field validations
            if let Some(obj) = value.as_object() {
                // Check for unknown fields (if strict mode)
                // TODO: Implement strict mode checking against schema
            }
        }

        // Custom validators
        for validator in &self.custom_validators {
            result.combine(validator(message_type, payload));
        }

        result
    }

    pub fn validate_order_payload(payload: &Value) -> ValidationResult {
        let mut result = ValidationResult::valid();

        // Validate side
        match payload.get("side").and_then(|v| v.as_str()) {
            Some("buy") | Some("sell") => {}
            Some(other) => result.add_error("side", "invalid_side",
                &format!("Invalid side: {}. Must be 'buy' or 'sell'", other)),
            None => result.add_error("side", "required", "Side is required"),
        }

        // Validate order type
        match payload.get("type").and_then(|v| v.as_str()) {
            Some(t) if ["market", "limit", "stop", "stop_limit"].contains(&t) => {}
            Some(other) => result.add_error("type", "invalid_type",
                &format!("Invalid order type: {}", other)),
            None => result.add_error("type", "required", "Order type is required"),
        }

        // Validate quantity
        if let Some(qty) = payload.get("quantity").and_then(|v| v.as_f64()) {
            if qty <= 0.0 {
                result.add_error("quantity", "invalid_quantity",
                    "Quantity must be positive");
            }
            if qty > 1000000.0 {
                result.add_error("quantity", "max_exceeded",
                    "Quantity exceeds maximum allowed");
            }
        } else {
            result.add_error("quantity", "required", "Quantity is required");
        }

        // Validate price for non-market orders
        match payload.get("type").and_then(|v| v.as_str()) {
            Some("market") => {}
            _ => {
                match payload.get("price").and_then(|v| v.as_f64()) {
                    Some(p) if p <= 0.0 => {
                        result.add_error("price", "invalid_price",
                            "Price must be positive");
                    }
                    None => {
                        result.add_error("price", "required",
                            "Price is required for non-market orders");
                    }
                    _ => {}
                }
            }
        }

        // Validate time in force
        let valid_tif = ["gtc", "ioc", "fok", "day", "gtd"];
        match payload.get("time_in_force").and_then(|v| v.as_str()) {
            Some(tif) if !valid_tif.contains(&tif) => {
                result.add_error("time_in_force", "invalid_tif",
                    &format!("Invalid time_in_force: {:?}. Must be one of {:?}", tif, valid_tif));
            }
            _ => {} // Optional field, defaults to GTC
        }

        result
    }

    pub fn validate_account_payload(payload: &Value) -> ValidationResult {
        let mut result = ValidationResult::valid();

        // Validate amount
        if let Some(amount) = payload.get("amount").and_then(|v| v.as_f64()) {
            if amount <= 0.0 {
                result.add_error("amount", "invalid_amount", "Amount must be positive");
            }
            if amount > 1000000000.0 {
                result.add_error("amount", "max_exceeded", "Amount exceeds maximum");
            }
        }

        // Validate currency
        if let Some(currency) = payload.get("currency").and_then(|v| v.as_str()) {
            let valid_currencies = ["USD", "EUR", "GBP", "BTC", "ETH", "USDT", "USDC"];
            if !valid_currencies.contains(&currency) {
                result.add_error("currency", "invalid_currency",
                    &format!("Unsupported currency: {}", currency));
            }
        }

        result
    }
}

// ---------------------------------------------------------------------------
// CONVENIENCE FUNCTIONS
// ---------------------------------------------------------------------------

pub fn validate_email(email: &str) -> bool {
    let re = regex::Regex::new(r"^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$").unwrap();
    re.is_match(email)
}

pub fn validate_phone(phone: &str) -> bool {
    let digits: String = phone.chars().filter(|c| c.is_ascii_digit()).collect();
    digits.len() >= 10 && digits.len() <= 15
}

pub fn validate_uuid(uuid: &str) -> bool {
    let re = regex::Regex::new(
        r"^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$"
    ).unwrap();
    re.is_match(uuid)
}

pub fn validate_hex_string(s: &str, expected_len: usize) -> bool {
    s.len() == expected_len * 2 && s.chars().all(|c| c.is_ascii_hexdigit())
}

pub fn validate_timestamp(ts: i64) -> bool {
    // Valid timestamps are between 2000-01-01 and 2100-01-01
    ts >= 946684800000 && ts <= 4102444800000
}

pub fn validate_symbol(symbol: &str) -> bool {
    let re = regex::Regex::new(r"^[A-Z0-9]{2,10}/[A-Z0-9]{2,10}$").unwrap();
    re.is_match(symbol)
}

pub fn validate_instrument_id(id: &str) -> bool {
    let re = regex::Regex::new(r"^[a-z0-9]{2,20}$").unwrap();
    re.is_match(id)
}

pub fn validate_price(price: f64) -> bool {
    price > 0.0 && price < 1_000_000_000.0 && (price * 1_000_000_000.0).fract() < 0.001
}

pub fn validate_quantity(qty: f64) -> bool {
    qty > 0.0 && qty < 100_000_000.0
}
