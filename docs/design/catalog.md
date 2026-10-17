# Behavior Specification

10 suites, 58 behaviors. Read from source; nothing was executed.

## BenchGate

### SaveAndAgainstNamingOneFile

| Behavior |
|----------|
| **when --save and --against name the same file** |
| &nbsp;&nbsp;is refused before anything runs, since the run would be compared against itself |

### AGateWithNothingToCompare

| Behavior |
|----------|
| **when no benchmark is in both the baseline and the run** |
| &nbsp;&nbsp;fails the gate and says there was nothing to compare |
| &nbsp;&nbsp;lists the baseline's benchmarks that did not run and the run's that the baseline lacks |

### AGateNeedsSamples

| Behavior |
|----------|
| **when --gate is given with -count below 4** |
| &nbsp;&nbsp;**-count absent** |
| &nbsp;&nbsp;&nbsp;&nbsp;exits 2 naming -count=4 as the requirement |
| &nbsp;&nbsp;**-count=2** |
| &nbsp;&nbsp;&nbsp;&nbsp;exits 2 naming -count=4 as the requirement |
| **when --gate is given with -count=4** |
| &nbsp;&nbsp;judges the run |

### AComparisonWithFewSamples

| Behavior |
|----------|
| **when only --against is given and a side has fewer than 4 samples** |
| &nbsp;&nbsp;still compares and marks each such row as a ±20% heuristic |

## CLIBuildRules

### LinknameIsCheckedAsGoTestChecksIt

| Behavior |
|----------|
| **when a package references a runtime symbol through a linkname the linker refuses** |
| &nbsp;&nbsp;fails to build with exit 2, as under go test |

## CLICoverageProfile

### ProfileSpellings

| Behavior |
|----------|
| **when a fully covered package runs with a minimum and a profile** |
| &nbsp;&nbsp;-coverprofile=file passes the gate |
| &nbsp;&nbsp;-coverprofile file passes the gate |
| &nbsp;&nbsp;summary -coverprofile=file prints the coverage |
| &nbsp;&nbsp;summary -coverprofile file prints the coverage |

## CLIFlags

### BooleanFlagSpellings

| Behavior |
|----------|
| **when a boolean flag is spelled in each accepted form** |
| &nbsp;&nbsp;--ci guards a focused run |
| &nbsp;&nbsp;--ci=true guards a focused run |
| &nbsp;&nbsp;--ci=false lets a focused run pass |
| &nbsp;&nbsp;the last occurrence wins |
| &nbsp;&nbsp;a non-boolean value is a usage error |
| &nbsp;&nbsp;summary --no-color=true prints no color codes |

### ValueFlagsAndUnknownFlags

| Behavior |
|----------|
| **when a value flag or an unknown flag is given to a command** |
| &nbsp;&nbsp;a value flag at the end without a value is a usage error |
| &nbsp;&nbsp;an unknown flag is a usage error for a run |
| &nbsp;&nbsp;an unknown flag is a usage error for clean |
| &nbsp;&nbsp;an unknown flag is a usage error for generate |
| &nbsp;&nbsp;an unknown flag is a usage error for discover |
| &nbsp;&nbsp;an unknown flag is a usage error for prepare |
| &nbsp;&nbsp;a flag another command owns is refused |

## CLIGoTestFlags

### OwnedGoTestFlags

| Behavior |
|----------|
| **when a go test flag gotest implements itself is given** |
| &nbsp;&nbsp;-run naming a fuzz wrapper selects its suite and replays the seeds |
| &nbsp;&nbsp;-exec wraps every suite binary |
| &nbsp;&nbsp;-p caps the processes and the run still completes |
| &nbsp;&nbsp;-o is refused with a reason |

## CLINewcomer

### ABrokenConfigStillHelps

| Behavior |
|----------|
| **when .gotest.yml does not parse** |
| &nbsp;&nbsp;version still prints |
| &nbsp;&nbsp;help config still prints |
| &nbsp;&nbsp;a run reports the config error |

### AMistypedCommandIsSuggested

| Behavior |
|----------|
| **when the first word is almost a subcommand or is a help topic** |
| &nbsp;&nbsp;sumary suggests summary |
| &nbsp;&nbsp;test suggests dropping the word |
| &nbsp;&nbsp;a package named like nothing runs as a pattern |

### AModuleWithoutGotestIsToldHow

| Behavior |
|----------|
| **when a suite imports gotest in a module that does not require it** |
| &nbsp;&nbsp;names the go get -tool command to run |

### StdlibTestsAreNotedInEveryMode

| Behavior |
|----------|
| **when a package has only stdlib tests** |
| &nbsp;&nbsp;the plain run notes them on stderr |
| &nbsp;&nbsp;the -json run notes them on stderr |
| &nbsp;&nbsp;summary notes them on stderr |
| &nbsp;&nbsp;spec notes them on stderr |

## CLIPassthrough

### UnknownFlagsReachTheBinary

| Behavior |
|----------|
| **when a flag only the test binary declares is passed** |
| &nbsp;&nbsp;after -- it reaches the binary and the pattern stays a package |
| &nbsp;&nbsp;after -args it reaches the binary verbatim |
| &nbsp;&nbsp;without it the binary's check fails |

## CLIScaffoldPaths

### FilePaths

| Behavior |
|----------|
| **when a source file is named by path** |
| &nbsp;&nbsp;a relative path scaffolds beside the file |
| &nbsp;&nbsp;an absolute path scaffolds beside the file |

## CLIStreams

### JSONOutputsStayPure

| Behavior |
|----------|
| **when a JSON document is asked for** |
| &nbsp;&nbsp;summary --format json inside GitHub Actions carries no annotations |
| &nbsp;&nbsp;summary --format json carries no color codes |
| &nbsp;&nbsp;spec --format json is one document |
| **when the go test -json stream is asked for** |
| &nbsp;&nbsp;holds JSON lines only, inside GitHub Actions too |

### ColorFollowsTheTerminal

| Behavior |
|----------|
| **when stdout is a pipe** |
| &nbsp;&nbsp;summary prints no color codes |
| &nbsp;&nbsp;summary --no-color=false still prints none |
| &nbsp;&nbsp;spec prints no color codes |
| &nbsp;&nbsp;NO_COLOR is honoured as well |

### TwoOutputModesAreRefused

| Behavior |
|----------|
| **when two output modes are asked for at once** |
| &nbsp;&nbsp;--spec with -json |
| &nbsp;&nbsp;summary with -json |
| &nbsp;&nbsp;an unknown --format |
| &nbsp;&nbsp;an unknown --format for spec |

## CLIVersion

### Version

| Behavior |
|----------|
| **when the version subcommand runs** |
| &nbsp;&nbsp;exits 0 and prints the version on stdout |

