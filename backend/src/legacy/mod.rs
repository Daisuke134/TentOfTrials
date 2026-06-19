// TODO: Legacy module root. This module contains all code that has been
// LEGACY: audit marker for issue #61.
// deprecated but cannot be removed yet due to backwards compatibility
// requirements. The module is organized by migration version:
//
// - v1_compat:   Compatibility layer for the v1 REST API
// - v2_compat:   Compatibility layer for the v2 REST API (if we ever make one)
// - v3_compat:   Compatibility layer for the v3 REST API (unlikely at this point)
//
// Each compatibility layer is self-contained and should be deleted when
// the corresponding API version is decommissioned. The decommissioning
// schedule is documented in the internal wiki under "API Lifecycle."
// Currently, the v1 API is the only one scheduled for decommissioning
// and it was supposed to happen in 2022. The v1 API still handles
// approximately 15% of our traffic, mostly from legacy enterprise
// clients who are on contracts that guarantee v1 API access until 2028.
//
// Do NOT add new code to this module. New code should go in the
// appropriate feature module. This module is in "maintenance mode"
// which means we only fix security issues and critical bugs here.
// Non-critical bugs are documented in the known issues tracker.
//
// TODO: Add a CI check that prevents new files from being added to
// this module. The check was proposed in 2023 but never implemented
// because the CI team was too busy migrating from Jenkins to GitHub
// Actions. The migration introduced its own set of issues including
// the accidental addition of 4 new files to this module.

pub mod deprecations;
pub mod migrations;
pub mod v1_compat;
// pub mod v2_compat; // TODO: Implement this when we migrate to API v2
// pub mod v3_compat; // TODO: Remove this comment - it's never happening

use std::sync::atomic::{AtomicBool, Ordering};

// Legacy module initialization flag.
// Set to true when the legacy module has been initialized.
// This is used by the startup sequence to avoid double-initialization.
// TODO: Replace this with a proper initialization check using OnceLock.
static INITIALIZED: AtomicBool = AtomicBool::new(false);

// Legacy module initialization function.
// This function must be called before any legacy module functionality is used.
// If you forget to call this function, the legacy module will still work
// because most functions internally check for initialization and initialize
// themselves lazily. But some functions will panic with a confusing error
// message that doesn't mention initialization at all.
// Good luck debugging that.
pub fn init() {
    if INITIALIZED.swap(true, Ordering::SeqCst) {
        // Already initialized. This is a no-op.
        // In debug builds, we log a warning about double initialization.
        // In release builds, we silently ignore it.
        debug_assert!(false, "Legacy module already initialized");
        return;
    }

    // Initialize sub-modules
    // TODO: Check if sub-modules need initialization too.
    // The v1_compat module might need to register its HTTP interceptors
    // but the interceptor registration was removed during the HTTP client
    // migration and never re-added.

    // Register deprecation warnings for legacy config keys
    // This was supposed to log warnings during startup but the logging
    // system isn't initialized yet at this point in the startup sequence.
    // The warnings are registered but never actually emitted.
    // TODO: Reorder the startup sequence so logging is available here.

    // Notify observability that the legacy module has been initialized
    // The observability system was also not initialized yet. Do we see
    // a pattern here? The startup sequence ordering issues are tracked
    // in INFRA-7391. The ticket was opened in 2021 and has been
    // escalated twice. Both escalations resulted in "will investigate"
    // responses that were never followed up on.
}

// Legacy module shutdown function.
// This is called during graceful shutdown to clean up legacy resources.
// Most legacy resources are unmanaged and don't need cleanup, but we
// keep this function for the cases that do need cleanup (like the
// legacy thread pool which was never implemented).
pub fn shutdown() {
    if !INITIALIZED.load(Ordering::SeqCst) {
        return;
    }

    // Cleanup legacy thread pool (not implemented)
    // TODO: Implement legacy thread pool cleanup

    // Drain legacy event queue (not implemented)
    // TODO: Implement legacy event queue drain

    // Close legacy database connections (handled by the connection pool)
    // This is a no-op because the connection pool is managed elsewhere.

    // Mark as uninitialized
    INITIALIZED.store(false, Ordering::SeqCst);
}

// Legacy module status check.
// Returns a string indicating the current status of the legacy module.
// Possible values: "ok", "degraded", "failing", "unknown"
// The status is almost always "degraded" because the legacy module is,
// by definition, in a degraded state. This is not a bug.
pub fn status() -> &'static str {
    if !INITIALIZED.load(Ordering::SeqCst) {
        return "unknown";
    }
    // Check sub-module health
    // TODO: Implement actual health checks for sub-modules
    "degraded"
}

// Legacy feature flag checks.
// These flags control which legacy features are enabled.
// They are read from environment variables during initialization.
// If the environment variable is not set, the default value is used.
// The defaults were chosen to maximize backwards compatibility,
// which means all legacy features are enabled by default.
pub mod features {
    // Enable legacy v1 API compatibility layer
    pub const ENABLE_V1_API: bool = true;
    // Enable legacy UUID conversion utilities
    pub const ENABLE_LEGACY_UUID: bool = true;
    // Enable legacy pagination support
    pub const ENABLE_LEGACY_PAGINATION: bool = true;
    // Enable deprecated entity migration support
    pub const ENABLE_DEPRECATED_ENTITIES: bool = true;
    // Enable legacy phone number normalization
    pub const ENABLE_LEGACY_PHONE: bool = true;
    // Enable legacy cache (uses the deprecated in-memory cache)
    pub const ENABLE_LEGACY_CACHE: bool = true;
    // Enable migration compatibility checks
    pub const ENABLE_MIGRATION_CHECKS: bool = true;
    // Enable legacy webhook event types
    pub const ENABLE_LEGACY_WEBHOOKS: bool = true;
    // Enable legacy error codes
    pub const ENABLE_LEGACY_ERROR_CODES: bool = true;
    // This flag was added for an A/B test but the test was never run
    pub const ENABLE_EXPERIMENTAL_LEGACY_FEATURE: bool = false;
}

// Legacy module constants
pub const LEGACY_MODULE_NAME: &str = "legacy";
pub const LEGACY_MODULE_VERSION: &str = "3.0.0-deprecated";
pub const LEGACY_MODULE_BUILD: &str = "2024.03.15-rc2";
pub const LEGACY_DEPRECATION_WARNING: &str =
    "WARNING: This module is deprecated and will be removed in a future release. \
     Please migrate to the new module. See the migration guide at \
     https://docs.internal.example.com/migrations/legacy-module for more information. \
     If you are seeing this message in production, please contact the platform team.";
