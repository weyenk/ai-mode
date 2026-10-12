package main

// This file is the home for the CLI flag-parsing implementation (task T01).
//
// During the test-first (red) phase the package-level seams `runCLI` and
// `runTimerFn` are already declared as stubs in the contract files
// (cli_contract.go and timer_types.go). The tests in cli_logic_test.go compile
// against those stubs and fail on their assertions. The real parsing logic
// covering scenarios S2-S6 will be implemented in this file.
//
// No declarations live here yet: redeclaring the seam variables would collide
// with the contract files and break the build.
