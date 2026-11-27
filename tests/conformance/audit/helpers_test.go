package audit_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/mvrahden/go-test/internal/testkit"
	"github.com/mvrahden/go-test/pkg/gotest"
)

// stage copies the fixture tree "base" or "head" into a fresh directory
// under parent and returns it.
func stage(t *gotest.T, parent, tree string) string {
	root, err := testkit.RepoRoot()
	gotest.NoError(t, err)
	dir := filepath.Join(parent, tree)
	gotest.NoError(t, testkit.StageModule(root, filepath.Join("..", "testdata", "audit", tree), dir))
	return dir
}

// record writes the record of the tree in dir to file and returns the run.
func record(t *gotest.T, dir, file string) testkit.Run {
	run, err := testkit.RunCLI(t.Context(), dir, "audit", "--record", "--output="+file)
	gotest.NoError(t, err)
	return run
}

// fixtureSum is the sha256 of a fixture file, read here rather than from a
// record.
func fixtureSum(t *gotest.T, tree, rel string) string {
	data, err := os.ReadFile(filepath.Join("..", "testdata", "audit", tree, filepath.FromSlash(rel)))
	gotest.NoError(t, err)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// auditRecord is the part of a record the suites read.
type auditRecord struct {
	Version  string `json:"version"`
	Schema   int    `json:"schema"`
	Packages []struct {
		Path       string   `json:"path"`
		Dir        string   `json:"dir"`
		Unreadable []string `json:"unreadable"`
		Suites     []struct {
			Name   string `json:"name"`
			File   string `json:"file"`
			Line   int    `json:"line"`
			Guard  bool   `json:"guard"`
			Budget struct {
				Timeout      string `json:"timeout"`
				SetupTimeout string `json:"setupTimeout"`
			} `json:"budget"`
			Methods []behavior `json:"methods"`
		} `json:"suites"`
	} `json:"packages"`
	Suppressions []struct {
		File   string   `json:"file"`
		Line   int      `json:"line"`
		Rules  []string `json:"rules"`
		Reason string   `json:"reason"`
	} `json:"suppressions"`
	Snapshots []struct {
		File   string `json:"file"`
		SHA256 string `json:"sha256"`
	} `json:"snapshots"`
}

type behavior struct {
	Kind       string     `json:"kind"`
	Name       string     `json:"name"`
	Display    string     `json:"display"`
	File       string     `json:"file"`
	Line       int        `json:"line"`
	EndLine    int        `json:"endLine"`
	Hash       string     `json:"hash"`
	Assertions int        `json:"assertions"`
	AssertsBy  bool       `json:"assertsBy"`
	Focused    bool       `json:"focused"`
	Excluded   bool       `json:"excluded"`
	Children   []behavior `json:"children"`
}

func readRecord(t *gotest.T, file string) auditRecord {
	data, err := os.ReadFile(file)
	gotest.NoError(t, err)
	var rec auditRecord
	gotest.NoError(t, json.Unmarshal(data, &rec), "record: %s", data)
	return rec
}

func flatOf(t *gotest.T, file string) map[string]behavior {
	rec := readRecord(t, file)
	return flat(&rec)
}

// flat lists a record's behaviors by their display path from the suite down.
func flat(rec *auditRecord) map[string]behavior {
	out := map[string]behavior{}
	var walk func(prefix string, bs []behavior)
	walk = func(prefix string, bs []behavior) {
		for i := range bs {
			path := prefix + " / " + bs[i].Display
			out[path] = bs[i]
			walk(path, bs[i].Children)
		}
	}
	for _, p := range rec.Packages {
		for _, s := range p.Suites {
			walk(s.Name, s.Methods)
		}
	}
	return out
}

// auditDoc is the part of an audit document the suites read.
type auditDoc struct {
	Findings []struct {
		Class   string `json:"class"`
		Weakens bool   `json:"weakens"`
		Subject string `json:"subject"`
		Base    string `json:"base"`
		Head    string `json:"head"`
	} `json:"findings"`
	Behaviors  []change `json:"behaviors"`
	Incomplete []string `json:"incomplete"`
	Base       struct {
		Ref    string `json:"ref"`
		Commit string `json:"commit"`
	} `json:"base"`
}

type change struct {
	Change string `json:"change"`
	Base   *place `json:"base"`
	Head   *place `json:"head"`
}

type place struct {
	Path []string `json:"path"`
	File string   `json:"file"`
	Line int      `json:"line"`
}

func readDoc(t *gotest.T, stdout string) auditDoc {
	var doc auditDoc
	gotest.NoError(t, json.Unmarshal([]byte(stdout), &doc), "document: %s", stdout)
	return doc
}

// finding is one finding as the suites compare it.
type finding struct{ Class, Subject, Base, Head string }

func findings(doc *auditDoc, weakens bool) []finding {
	var out []finding
	for _, f := range doc.Findings {
		if f.Weakens == weakens {
			out = append(out, finding{f.Class, f.Subject, f.Base, f.Head})
		}
	}
	return out
}
