package withnolintreason

import "testing"

// A want comment sits before each nolint comment: one after it would read
// as its reason.

func TestBare(t *testing.T) {} /* want "nolint comment gives no reason" */ //nolint

func TestSpaced(t *testing.T) {} /* want "nolint comment gives no reason" */ // nolint:stdlib-test

func TestAll(t *testing.T) {} /* want "nolint comment gives no reason" */ //nolint:all

func TestEmptyReason(t *testing.T) {} /* want "nolint comment gives no reason" */ //nolint:stdlib-test //

func TestBlankReason(t *testing.T) {} /* want "nolint comment gives no reason" */ //nolint //

func TestItself(t *testing.T) {} /* want "nolint comment gives no reason" */ //nolint:nolint-reason,stdlib-test

func TestReasoned(t *testing.T) {} //nolint:stdlib-test // kept as a stdlib test

func TestBareReasoned(t *testing.T) {} //nolint // kept as a stdlib test

func TestForeign(t *testing.T) {} /* want "stdlib test TestForeign" */ //nolint:gosec
