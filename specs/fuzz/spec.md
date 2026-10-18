# Fuzz Specification

## Purpose
How a suite's Fuzz methods run their executions, and how each execution's BeforeEach and AfterEach pair on every exit path.

## Requirements

### Requirement: Execution hooks pair
A fuzz execution whose BeforeEach fails or panics SHALL fail, and its AfterEach SHALL still run.

#### Scenario: BeforeEach fails
- **WHEN** a fuzz execution's BeforeEach fails
- **THEN** the execution fails and its AfterEach still runs

#### Scenario: BeforeEach panics
- **WHEN** a fuzz execution's BeforeEach panics
- **THEN** the execution fails and its AfterEach still runs
