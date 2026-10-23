# Bench Specification

## Purpose
How `gotest bench` compares a run against a saved baseline, when its gate passes, fails or refuses to judge, and what an expired --timeout names.

## Requirements

### Requirement: The baseline is not the run's own output
A bench run SHALL refuse, before anything runs, to save to the file it compares against.

#### Scenario: Saving to the baseline it compares against
- **WHEN** --save and --against name the same file
- **THEN** is refused before anything runs, since the run would be compared against itself

### Requirement: A gate with nothing to compare
A gate SHALL fail when no benchmark is in both the baseline and the run, and SHALL list the benchmarks each side lacks.

#### Scenario: Baseline and run share no benchmark
- **WHEN** no benchmark is in both the baseline and the run
- **THEN** fails the gate and says there was nothing to compare
- **AND** lists the baseline's benchmarks that did not run and the run's that the baseline lacks

### Requirement: A gate needs four samples
A gate SHALL judge a run only with -count=4 or more, and SHALL exit 2 naming that requirement below it.

#### Scenario: Too few samples for a gate
- **WHEN** --gate is given with -count below 4
- **THEN** exits 2 naming -count=4 as the requirement

#### Scenario: Enough samples for a gate
- **WHEN** --gate is given with -count=4
- **THEN** judges the run

### Requirement: A comparison without a gate
A comparison without a gate SHALL compare whatever the sample count, and SHALL mark each row a side has fewer than 4 samples for as a ±20% heuristic.

#### Scenario: A gate of zero
- **WHEN** --gate=0 is given
- **THEN** compares without a gate, whatever -count says

#### Scenario: A comparison with few samples
- **WHEN** only --against is given and a side has fewer than 4 samples
- **THEN** still compares and marks each such row as a ±20% heuristic

### Requirement: Words for the test binary
A bench run SHALL put its own flags ahead of the words passed after -args.

#### Scenario: Words after -args
- **WHEN** a bench run passes words after -args
- **THEN** puts its own -v ahead of them and the run passes

### Requirement: What an expired --timeout names in a bench run
An expired --timeout SHALL name the benchmark it cut short, in a text bench run as in --json, not only the suite that runs it.

#### Scenario: A benchmark that hangs
- **WHEN** a benchmark hangs until --timeout in a text bench run
- **THEN** names the benchmark as running when the timeout expired
