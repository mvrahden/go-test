# Assertions Specification

## Purpose
What gotest's assertions, polls and snapshots accept, where their failures point, and what their failure messages say.

## Requirements

### Requirement: Failure locations
An assertion that fails inside a helper SHALL name the helper's line and the caller's, also in a module whose path's first element has no dot, built with -trimpath, and also when the helper is in another such module the build requires.

#### Scenario: A helper in a dotless module under -trimpath
- **WHEN** a helper fails in a module whose path has no dot, built with -trimpath
- **THEN** the failure names the helper's line and the caller's

#### Scenario: A helper in a module whose later path element has a dot, under -trimpath
- **WHEN** a helper fails in a module whose path's first element has no dot and a later one has, built with -trimpath
- **THEN** the failure names the helper's line and the caller's

#### Scenario: A helper in a required dotless module under -trimpath
- **WHEN** a behavior another module declares fails in its helper, the module's path having no dot, required through a replace and built with -trimpath
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
Contains and NotContains SHALL fail, naming the haystack's type and the needle's, when the needle could be no element or key of the haystack, and SHALL name the type to convert it to only when the conversion keeps its value: a string kind to a string kind, a number that converts to the element type and back unchanged, sign included, and a rune, byte or byte slice to the string it is looked for in.

#### Scenario: A needle the haystack cannot hold
- **WHEN** Contains is given a needle no element of the haystack could be
- **THEN** fails naming the haystack's type and the needle's

#### Scenario: A needle that does not implement the element interface
- **WHEN** NotContains is given a needle that does not implement the interface the slice's elements are
- **THEN** fails naming the slice's type and the needle's

#### Scenario: A number of another numeric type
- **WHEN** Contains is given a number of another type than the slice's numeric elements
- **THEN** the failure names the type to convert it to

#### Scenario: A number the elements cannot hold exactly
- **WHEN** Contains is given a number the slice's numeric elements cannot hold exactly
- **THEN** the failure names no type to convert it to

#### Scenario: A rune, byte or byte slice in a string
- **WHEN** Contains is given a rune, a byte or a byte slice for a string
- **THEN** fails naming the string type to convert it to

#### Scenario: A needle a map cannot hash
- **WHEN** Contains is given a needle that is not comparable for a map keyed by an interface type
- **THEN** fails naming the needle's type

### Requirement: A nil needle
Contains SHALL take a nil needle as the nil element or key of a slice, array or map whose element or key type can be nil, and SHALL fail naming the haystack's type when it cannot be nil, a string's substring included.

#### Scenario: A map whose keys can be nil
- **WHEN** Contains is asked for a nil key in a map keyed by an interface or pointer type
- **THEN** finds the key when present and reports it absent otherwise

#### Scenario: A map whose keys cannot be nil
- **WHEN** Contains is asked for a nil key in a map whose key type cannot be nil
- **THEN** fails naming the map's type

#### Scenario: A slice whose elements can be nil
- **WHEN** Contains is asked for a nil element in a slice of a pointer type
- **THEN** finds the nil element when present and reports it absent otherwise

#### Scenario: A slice whose elements cannot be nil
- **WHEN** Contains is asked for a nil element in a slice whose element type cannot be nil
- **THEN** fails naming the slice's type

#### Scenario: A nil substring
- **WHEN** Contains is asked for a nil substring of a string
- **THEN** fails naming the string's type

### Requirement: InDelta at the edges of float64
InDelta SHALL fail when the delta is NaN, and SHALL hold two equal infinities within any delta.

#### Scenario: A NaN delta
- **WHEN** InDelta is given a NaN delta
- **THEN** fails: no distance is within NaN

#### Scenario: Equal infinities
- **WHEN** InDelta compares two equal infinities
- **THEN** they are within any delta

### Requirement: InDelta on integers
InDelta SHALL measure the distance between two integers exactly, neither rounding them through float64 nor overflowing their type.

#### Scenario: Integers past float64's exact range
- **WHEN** InDelta compares integers beyond float64's exact range
- **THEN** 9007199254740993 is not within a delta of 0 of 9007199254740992

#### Scenario: Integers at opposite ends of their type
- **WHEN** InDelta compares the smallest and the largest int64
- **THEN** measures their full distance, not one that wrapped around

### Requirement: Failure messages
A failure message SHALL format the extra arguments by the first when it is of any string kind and carry every extra argument when it is not, and ErrorAs SHALL name the type it expected when the error is of another type.

#### Scenario: Extra arguments without a format
- **WHEN** an assertion's first extra argument is not a string
- **THEN** every extra argument is in the message

#### Scenario: A format of a named string type
- **WHEN** an assertion's first extra argument is of a named string type
- **THEN** formats the arguments after it

#### Scenario: ErrorAs on another error type
- **WHEN** ErrorAs is given an error of another type
- **THEN** names the type it expected

### Requirement: JSON numbers
JSONEq SHALL compare numbers by value: integers beyond float64's exact range stay distinct, an integer equals its decimal spelling at any exponent, and -0 equals 0.

#### Scenario: Integers past float64's exact range
- **WHEN** JSONEq compares integers beyond float64's exact range
- **THEN** tells 9007199254740993 from 9007199254740992

#### Scenario: An integer and its decimal spelling
- **WHEN** JSONEq compares an integer with its decimal spelling
- **THEN** 1 equals 1.0

#### Scenario: Exponents beyond any float
- **WHEN** JSONEq compares numbers whose exponents are spelled differently, however large
- **THEN** 1e10000000 equals 10e9999999

#### Scenario: Negative zero
- **WHEN** JSONEq compares -0 with 0
- **THEN** they are equal

### Requirement: One JSON value
JSONEq SHALL fail when an input holds anything but whitespace after its JSON value.

#### Scenario: A stray closing bracket
- **WHEN** JSONEq is given a value followed by a stray } or ]
- **THEN** fails: the input is not valid JSON

### Requirement: Mismatch diffs
When Equal fails on values that do not each render on one short line, its message SHALL render both one field, element or entry per line and SHALL mark the lines that differ under "diff (-expected +actual):" in hunks with three lines of context, without file headers or line numbers, each hunk led by the path of its first change; that diff SHALL show a final newline only one side ends in, and a long changed line around its first difference, marking a missing final newline on it too.

#### Scenario: Values that render on several lines
- **WHEN** Equal fails on values whose rendering spans lines
- **THEN** the message carries a line diff: hunks of changes with three lines of context, without file headers

#### Scenario: A final newline on one side
- **WHEN** Equal fails on multi-line values that differ only in a final newline
- **THEN** the diff marks the side that does not end in one

#### Scenario: A nested field
- **WHEN** Equal fails on structs that differ in one nested field
- **THEN** the diff marks that field's line and no other

#### Scenario: Where a hunk is
- **WHEN** Equal fails on values that differ in a field of an element
- **THEN** the hunk is led by the path of its first changed line, such as at [1].Name:

#### Scenario: A deleted element
- **WHEN** Equal fails on slices of structs one of which lacks an element
- **THEN** the diff marks that element from its opening line to its closing one

#### Scenario: A long changed line
- **WHEN** Equal fails on values whose one changed line is longer than 80 bytes
- **THEN** the diff shows that line around its first difference with a caret under it

#### Scenario: A long last line without a final newline
- **WHEN** Equal fails on multi-line values whose changed last line is longer than 80 bytes and ends in a newline on one side only
- **THEN** the diff shows that line around its first difference and marks the side without the newline

#### Scenario: A changed row of hex bytes
- **WHEN** Equal fails on byte slices that are not UTF-8 and differ inside a row
- **THEN** the hunk names the first differing byte, such as at [12]:, not the start of its row

### Requirement: Renderings
A rendering SHALL follow pointers, include unexported fields and show no addresses; SHALL order map entries by key, and keys that render alike by their values; SHALL split text at its newlines, show bytes that are not text in hex, values of package sync by their type alone and unsigned integers in decimal; SHALL indent no deeper than level 32, naming the depth of the lines below it; and SHALL stop at 50,000 lines or 4 MiB, naming where the values first differ.

#### Scenario: Elements holding pointers
- **WHEN** Equal fails on slices whose elements hold distinct pointers to equal values and one element differs
- **THEN** the diff marks a line of that element only

#### Scenario: An unexported field
- **WHEN** Equal fails on structs that differ only in an unexported field
- **THEN** the diff marks that field's line

#### Scenario: Text that spans lines
- **WHEN** Equal fails on strings that span lines
- **THEN** the diff marks the changed lines of text

#### Scenario: Keys that render alike
- **WHEN** Equal fails on maps whose keys render alike, such as distinct pointers to equal values
- **THEN** entries are ordered by their values and only the changed entry is marked, the same way every time

#### Scenario: Byte slices of text
- **WHEN** Equal fails on byte slices of UTF-8 text that span lines
- **THEN** they render as []byte("…") split at their newlines and the diff marks the changed lines

#### Scenario: Byte slices that are not text
- **WHEN** Equal fails on byte slices that are not UTF-8
- **THEN** they render as rows of eight hex bytes

#### Scenario: Values of package sync
- **WHEN** Equal fails on structs that hold a sync.Mutex and differ in another field
- **THEN** the mutex renders as sync.Mutex{…}

#### Scenario: Unsigned integers
- **WHEN** Equal fails on values holding unsigned integers
- **THEN** they render in decimal, a uintptr in hex

#### Scenario: Deep values
- **WHEN** Equal fails on values nested more than 32 levels deep
- **THEN** lines deeper than 32 levels keep that level's indent and name their depth

#### Scenario: Values too large to render
- **WHEN** Equal fails on values whose renderings pass 50,000 lines and agree up to there
- **THEN** the message says where they are cut and names the path of their first difference

### Requirement: Mismatches without a diff
When Equal fails on values that each render on one short line, its message SHALL show both and no diff; on two long single-line strings, or byte slices of text, it SHALL name the byte offset of their first difference and show both around it, with a caret under it that counts a wide character as two columns; on other values that each render on one line, one of them longer than 160 bytes, it SHALL show both around their first difference with a caret under it.

#### Scenario: Short values
- **WHEN** Equal fails on values that each render on one short line
- **THEN** the message shows both values and no diff

#### Scenario: Long strings
- **WHEN** Equal fails on two long single-line strings
- **THEN** the message names the byte offset of the first difference and shows both strings around it

#### Scenario: Long byte slices of text
- **WHEN** Equal fails on two long single-line byte slices of UTF-8 text
- **THEN** the message names the byte offset of the first difference and shows both around it

#### Scenario: Wide characters
- **WHEN** Equal fails on two long strings with wide characters before their first difference
- **THEN** the caret sits under the difference, a wide character counting two columns

#### Scenario: Long single-line renderings
- **WHEN** Equal fails on values that each render on one line, one longer than 160 bytes, that first differ past byte 160
- **THEN** the message shows both around their first difference with a caret under it

### Requirement: Values that look alike
When Equal fails on values of different dynamic types, its message SHALL name both types and, for long single-line text of one kind, show the text as two long strings are shown; on values whose renderings are identical it SHALL show the difference of their raw fields or, when those render alike too, say where the values differ and that the rendering does not show it instead of showing the value twice. No Equal or NotEqual message SHALL show an address.

#### Scenario: Different dynamic types
- **WHEN** Equal fails on short values of different dynamic types
- **THEN** the message names both types and shows each value in its type

#### Scenario: Long strings of different types
- **WHEN** Equal fails on two long single-line strings of different types inside any
- **THEN** the message names both types, then the byte offset of the first difference and both strings around it

#### Scenario: GoString methods that render alike
- **WHEN** Equal fails on values whose GoString methods render them alike
- **THEN** the message shows the difference of their raw fields

#### Scenario: A difference the rendering hides
- **WHEN** Equal fails on values that render alike, such as two NaNs beside distinct pointers to equal values
- **THEN** the message names where they differ, says the rendering does not show it and shows the value once, without addresses

#### Scenario: NotEqual on values holding pointers
- **WHEN** NotEqual fails on values holding pointers
- **THEN** the message shows the value they share without addresses

### Requirement: Polls and their deadline
Eventually and Consistently SHALL judge a poll that started before waitFor by its result, even when it returns after waitFor and after many ticks; SHALL give a poll in flight at the deadline until waitFor past it, and fail, saying the poll is still running past its deadline, when it has not returned by then; and SHALL run a final poll at the deadline when no poll is in flight as it passes.

#### Scenario: An Eventually poll that hangs
- **WHEN** an Eventually poll runs past waitFor
- **THEN** fails no later than waitFor past the deadline and says the poll is still running past its deadline

#### Scenario: A Consistently poll that hangs
- **WHEN** a Consistently poll runs past waitFor
- **THEN** fails no later than waitFor past the deadline and says the poll is still running past its deadline

#### Scenario: A passing Consistently poll that ends after waitFor
- **WHEN** every Consistently poll passes but one runs past waitFor
- **THEN** passes

#### Scenario: An Eventually poll that passes after waitFor
- **WHEN** an Eventually poll started before waitFor passes after it
- **THEN** passes

#### Scenario: A Consistently poll slower than tick
- **WHEN** a passing Consistently poll in flight at the deadline takes many ticks
- **THEN** passes

#### Scenario: An Eventually final poll slower than tick
- **WHEN** the final Eventually poll passes after many ticks
- **THEN** passes

#### Scenario: A tick longer than waitFor
- **WHEN** tick is longer than waitFor and the first poll fails
- **THEN** a final poll runs at the deadline and can still pass

#### Scenario: A tick longer than waitFor for Consistently
- **WHEN** tick is longer than waitFor and every Consistently poll passes
- **THEN** a final poll runs at the deadline

### Requirement: Abandoned polls
A poll abandoned past its deadline SHALL keep running, since Go cannot stop it, and a panic it raises later SHALL be written to stderr with its stack instead of failing the test, which has already failed.

#### Scenario: A late panic
- **WHEN** a poll abandoned past its deadline panics later
- **THEN** the panic is written to stderr with its stack

### Requirement: Polling arguments
Eventually and Consistently SHALL fail without polling when waitFor or tick is not positive.

#### Scenario: A zero or negative duration
- **WHEN** waitFor or tick is not positive
- **THEN** fails saying both must be positive and polls nothing

### Requirement: Snapshot sections
MatchSnapshot SHALL key each section by the exact subtest name and SHALL find the baselines beside the test, also when the package is built with -trimpath. Under -trimpath the caller's file cannot be opened and the package directory go test runs in stands in for it, so a snapshot taken in a helper of another package is found beside the package under test, not beside the helper.

#### Scenario: Each rows and twin subtests
- **WHEN** Each rows and same-named subtests match snapshots
- **THEN** #0, #1, x and x#01 each read their own section

#### Scenario: A build under -trimpath
- **WHEN** the package is built with -trimpath
- **THEN** the baselines are still found beside the test, _ext suffix included

### Requirement: Baselines keyed by the release before exact names
When a section's exact key is missing, MatchSnapshot SHALL compare against the section under the key the release before exact names derived (a trailing #<digits> stripped), unless another test's exact key or another migration claimed it this run. Under GOTEST_CI it SHALL leave the file as it was; otherwise it SHALL rename a matching section to the exact key. Benchmark and fuzz snapshots, which that release filed under unknown.snap as _, SHALL NOT be migrated.

#### Scenario: An old-keyed baseline
- **WHEN** a baseline is keyed the way the release before exact names keyed it
- **THEN** its content is compared under the new key
- **AND** under GOTEST_CI the file is left as it was
- **AND** otherwise a matching section is renamed to the new key

#### Scenario: An old key another test holds
- **WHEN** the old key of a section is the exact key of another test
- **THEN** the section is not reused for a second key

### Requirement: Snapshot updates switched off
GOTEST_UPDATE_SNAPSHOTS and GOTEST_CI SHALL each be off when unset, empty or set to a false spelling of strconv.ParseBool (0, f, F, false, FALSE, False), and on for any other value, a mistyped one included. With GOTEST_UPDATE_SNAPSHOTS off, a stale snapshot SHALL fail and its baseline file SHALL stay unchanged.

#### Scenario: Updates disabled
- **WHEN** GOTEST_UPDATE_SNAPSHOTS is 0
- **THEN** a stale baseline still fails and is left as it was

#### Scenario: Other false spellings
- **WHEN** GOTEST_CI and GOTEST_UPDATE_SNAPSHOTS are spelled FALSE and f
- **THEN** both are off: a missing baseline is written and a stale one still fails

### Requirement: Snapshot line endings
MatchSnapshot SHALL read every carriage return before a newline, and those at the end of the value, as part of the line ending, so a baseline written for such a value matches it again.

#### Scenario: Carriage returns at line ends
- **WHEN** a value ends lines with runs of carriage returns, or ends in one
- **THEN** matches the baseline written with plain newlines

### Requirement: Snapshot names
MatchSnapshot SHALL refuse a name that spans lines or spells a section header, before it writes anything.

#### Scenario: A name that could forge a section
- **WHEN** a snapshot name spans lines or spells a section header
- **THEN** is refused before anything is written
