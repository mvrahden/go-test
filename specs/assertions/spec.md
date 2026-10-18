# Assertions Specification

## Purpose
What gotest's assertions, polls and snapshots accept, where their failures point, and what their failure messages say.

## Requirements

### Requirement: Failure locations
An assertion that fails inside a helper SHALL name the helper's line and the caller's, also in a module whose path has no dot, built with -trimpath.

#### Scenario: A helper in a dotless module under -trimpath
- **WHEN** a helper fails in a module whose path has no dot, built with -trimpath
- **THEN** the failure names the helper's line and the caller's

### Requirement: Named string types
Regexp and Contains SHALL treat a value of a named string type as the string it holds.

#### Scenario: A named pattern type
- **WHEN** Regexp is given a pattern of a named string type
- **THEN** compiles and matches it like a string

#### Scenario: Named string haystack and needle
- **WHEN** Contains compares named string kinds
- **THEN** checks the substring as it does for strings

### Requirement: A needle of the wrong type
Contains and NotContains SHALL fail, naming the haystack's type and the needle's, when the needle could be no element or key of the haystack.

#### Scenario: A needle the haystack cannot hold
- **WHEN** Contains is given a needle no element of the haystack could be
- **THEN** fails naming the haystack's type and the needle's

### Requirement: A nil map key
Contains SHALL look up a nil needle as the nil key of a map keyed by an interface or pointer type, and SHALL fail naming the map's type when the map's key type cannot be nil.

#### Scenario: A map whose keys can be nil
- **WHEN** Contains is asked for a nil key in a map keyed by an interface or pointer type
- **THEN** finds the key when present and reports it absent otherwise

#### Scenario: A map whose keys cannot be nil
- **WHEN** Contains is asked for a nil key in a map whose key type cannot be nil
- **THEN** fails naming the map's type

### Requirement: InDelta at the edges of float64
InDelta SHALL fail when the delta is NaN, and SHALL hold two equal infinities within any delta.

#### Scenario: A NaN delta
- **WHEN** InDelta is given a NaN delta
- **THEN** fails: no distance is within NaN

#### Scenario: Equal infinities
- **WHEN** InDelta compares two equal infinities
- **THEN** they are within any delta

### Requirement: Failure messages
A failure message SHALL carry every extra argument when the first is not a string, and ErrorAs SHALL name the type it expected when the error is of another type.

#### Scenario: Extra arguments without a format
- **WHEN** an assertion's first extra argument is not a string
- **THEN** every extra argument is in the message

#### Scenario: ErrorAs on another error type
- **WHEN** ErrorAs is given an error of another type
- **THEN** names the type it expected

### Requirement: JSON numbers
JSONEq SHALL compare numbers by value: integers beyond float64's exact range stay distinct, and an integer equals its decimal spelling.

#### Scenario: Integers past float64's exact range
- **WHEN** JSONEq compares integers beyond float64's exact range
- **THEN** tells 9007199254740993 from 9007199254740992

#### Scenario: An integer and its decimal spelling
- **WHEN** JSONEq compares an integer with its decimal spelling
- **THEN** 1 equals 1.0

### Requirement: One JSON value
JSONEq SHALL fail when an input holds anything but whitespace after its JSON value.

#### Scenario: A stray closing bracket
- **WHEN** JSONEq is given a value followed by a stray } or ]
- **THEN** fails: the input is not valid JSON

### Requirement: Multi-line diffs
When Equal fails on values whose rendering spans lines, its message SHALL carry a unified diff whose hunks hold three lines of context.

#### Scenario: Values that render on several lines
- **WHEN** Equal fails on values whose rendering spans lines
- **THEN** the message carries a unified diff: hunks of changes with three lines of context

### Requirement: Polls and their deadline
Eventually and Consistently SHALL judge a poll that started before waitFor by its result, even when it returns after waitFor; SHALL fail, saying the poll is still running, when a poll has not returned one tick past waitFor; and Eventually SHALL run a final poll at the deadline when tick is longer than waitFor.

#### Scenario: An Eventually poll that hangs
- **WHEN** an Eventually poll runs past waitFor
- **THEN** fails no later than one tick past waitFor and says the poll is still running

#### Scenario: A Consistently poll that hangs
- **WHEN** a Consistently poll runs past waitFor
- **THEN** fails no later than one tick past waitFor and says the poll is still running

#### Scenario: A passing Consistently poll that ends after waitFor
- **WHEN** every Consistently poll passes but one runs past waitFor
- **THEN** passes

#### Scenario: An Eventually poll that passes after waitFor
- **WHEN** an Eventually poll started before waitFor passes after it
- **THEN** passes

#### Scenario: A tick longer than waitFor
- **WHEN** tick is longer than waitFor and the first poll fails
- **THEN** a final poll runs at the deadline and can still pass

### Requirement: Polling arguments
Eventually and Consistently SHALL fail without polling when waitFor or tick is not positive.

#### Scenario: A zero or negative duration
- **WHEN** waitFor or tick is not positive
- **THEN** fails with a type-guard message and polls nothing

### Requirement: Snapshot sections
MatchSnapshot SHALL key each section by the exact subtest name and SHALL find the baselines beside the test, also when the package is built with -trimpath.

#### Scenario: Each rows and twin subtests
- **WHEN** Each rows and same-named subtests match snapshots
- **THEN** #0, #1, x and x#01 each read their own section

#### Scenario: A build under -trimpath
- **WHEN** the package is built with -trimpath
- **THEN** the baselines are still found beside the test, _ext suffix included

### Requirement: Snapshot updates switched off
With GOTEST_UPDATE_SNAPSHOTS set to 0, a stale snapshot SHALL fail and its baseline file SHALL stay unchanged.

#### Scenario: Updates disabled
- **WHEN** GOTEST_UPDATE_SNAPSHOTS is 0
- **THEN** a stale baseline still fails and is left as it was

### Requirement: Snapshot names
MatchSnapshot SHALL refuse a name that spans lines or spells a section header, before it writes anything.

#### Scenario: A name that could forge a section
- **WHEN** a snapshot name spans lines or spells a section header
- **THEN** is refused before anything is written
