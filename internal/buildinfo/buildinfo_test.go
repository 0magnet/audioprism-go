package buildinfo

import "testing"

// save and restore the package globals parseVersionInfo writes into, so one
// test does not leave another reading a version it never set.
func withCleanGlobals(t *testing.T) {
	t.Helper()
	v, c, d := version, commit, date
	t.Cleanup(func() { version, commit, date = v, c, d })
	version, commit, date = unknown, unknown, unknown
}

func TestFormatBuildDate(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"20250410212328", "2025-04-10T21:23:28Z"},
		{"19700101000000", "1970-01-01T00:00:00Z"},
		{"99991231235959", "9999-12-31T23:59:59Z"},
	} {
		if got := formatBuildDate(tc.in); got != tc.want {
			t.Errorf("formatBuildDate(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Anything that is not fourteen digits is not a timestamp, and guessing at one
// would put a wrong date on a release.
func TestFormatBuildDateRejectsTheWrongLength(t *testing.T) {
	for _, in := range []string{"", "2025", "202504102123", "202504102123280", "not a date!!!!"} {
		if got := formatBuildDate(in); got != unknown {
			t.Errorf("formatBuildDate(%q) = %q, want %q", in, got, unknown)
		}
	}
}

// The pseudo-version Go generates is version, timestamp and commit run
// together. Each has to come back out into its own field.
func TestParseVersionInfoSplitsAPseudoVersion(t *testing.T) {
	withCleanGlobals(t)
	parseVersionInfo("v1.3.29-rc7.0.20250410212328-dc5d22b7ab2a")

	if commit != "dc5d22b7ab2a" {
		t.Errorf("commit = %q, want dc5d22b7ab2a", commit)
	}
	if date != "2025-04-10T21:23:28Z" {
		t.Errorf("date = %q, want the formatted timestamp", date)
	}
	if version != "v1.3.29-rc7" {
		t.Errorf("version = %q, want v1.3.29-rc7 with the timestamp and commit taken out", version)
	}
}

// A plain tag has neither a timestamp nor a commit in it, and must survive
// untouched rather than being trimmed at.
func TestParseVersionInfoOnAPlainTag(t *testing.T) {
	withCleanGlobals(t)
	parseVersionInfo("v1.2.3")

	if version != "v1.2.3" {
		t.Errorf("version = %q, want v1.2.3", version)
	}
	if commit != unknown {
		t.Errorf("commit = %q, want it left unknown", commit)
	}
	if date != unknown {
		t.Errorf("date = %q, want it left unknown", date)
	}
}

func TestParseVersionInfoWithACommitButNoDate(t *testing.T) {
	withCleanGlobals(t)
	parseVersionInfo("v0.0.0-dc5d22b7ab2a")

	if commit != "dc5d22b7ab2a" {
		t.Errorf("commit = %q", commit)
	}
	if version != "v0.0.0" {
		t.Errorf("version = %q, want v0.0.0", version)
	}
}

// The commit is matched at the end of the string, so a hex-looking run in the
// middle of a version is not mistaken for one.
func TestParseVersionInfoOnlyTakesACommitFromTheEnd(t *testing.T) {
	withCleanGlobals(t)
	parseVersionInfo("vdeadbeefcafe-1.0.0")
	if commit != unknown {
		t.Errorf("commit = %q, want it left unknown — the hex run is not at the end", commit)
	}
}

// Version reports the fallback rather than Go's placeholder, which is what
// `go run` and an untagged build produce.
func TestVersionReportsUnknownForAPlaceholder(t *testing.T) {
	withCleanGlobals(t)
	for _, in := range []string{"(devel)", ""} {
		version = in
		if got := Version(); got != unknown {
			t.Errorf("with version %q, Version() = %q, want %q", in, got, unknown)
		}
	}
}

func TestVersionReportsWhatWasParsed(t *testing.T) {
	withCleanGlobals(t)
	version = "v9.9.9"
	if got := Version(); got != "v9.9.9" {
		t.Errorf("Version() = %q, want v9.9.9", got)
	}
}

// The accessors are what the CLI prints, so none of them may come back empty:
// a blank line under `--version` is worse than the word unknown.
func TestAccessorsNeverReturnEmpty(t *testing.T) {
	for name, fn := range map[string]func() string{
		"Version": Version,
		"Commit":  Commit,
		"Date":    Date,
		"Go":      Go,
	} {
		if got := fn(); got == "" {
			t.Errorf("%s() is empty", name)
		}
	}
}
