#!/usr/bin/env python3
"""Unit tests for module selection validation helpers."""

import pytest
from build import Module, parse_module_names, resolve_modules

ROOT = "."

SAMPLE_MODULES = [
    Module(name="backend",    language="Rust",       dir=".", build_cmd=[], clean_cmd=[]),
    Module(name="frontend",   language="TypeScript", dir=".", build_cmd=[], clean_cmd=[]),
    Module(name="market",     language="Go",         dir=".", build_cmd=[], clean_cmd=[]),
    Module(name="frailbox",   language="C",          dir=".", build_cmd=[], clean_cmd=[]),
]


class TestParseModuleNames:
    def test_single_name(self):
        assert parse_module_names("backend") == ["backend"]

    def test_comma_separated(self):
        assert parse_module_names("backend,frontend") == ["backend", "frontend"]

    def test_comma_with_spaces(self):
        assert parse_module_names("backend, frontend") == ["backend", "frontend"]

    def test_trailing_whitespace(self):
        assert parse_module_names("  backend , frontend  ") == ["backend", "frontend"]

    def test_empty_string(self):
        assert parse_module_names("") == []

    def test_all_keyword(self):
        # The "all" keyword is handled specially by main(), not by this helper
        assert parse_module_names("all") == ["all"]


class TestResolveModules:
    def test_valid_single(self):
        matched, not_found = resolve_modules(["backend"], SAMPLE_MODULES)
        assert len(matched) == 1
        assert matched[0].name == "backend"
        assert not_found == set()

    def test_valid_multiple(self):
        matched, not_found = resolve_modules(["backend", "frontend"], SAMPLE_MODULES)
        assert len(matched) == 2
        assert not_found == set()

    def test_invalid_name(self):
        matched, not_found = resolve_modules(["nope"], SAMPLE_MODULES)
        assert matched == []
        assert not_found == {"nope"}

    def test_mixed_valid_and_invalid(self):
        matched, not_found = resolve_modules(["backend", "nope", "frontend"], SAMPLE_MODULES)
        assert len(matched) == 2
        assert not_found == {"nope"}

    def test_all_empty(self):
        matched, not_found = resolve_modules([], SAMPLE_MODULES)
        assert matched == []
        assert not_found == set()

    def test_case_sensitive(self):
        matched, not_found = resolve_modules(["Backend"], SAMPLE_MODULES)
        assert matched == []
        assert not_found == {"Backend"}


if __name__ == "__main__":
    pytest.main([__file__, "-v"])
