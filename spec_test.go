package spec

import (
	"testing"

	"github.com/alecthomas/kingpin/v2"
	"github.com/carapace-sh/carapace-spec/pkg/command"
)

func TestCommandDefaultValues(t *testing.T) {
	app := kingpin.New("app", "test app")
	app.Flag("output", "output path").Default("/tmp/out.txt").String()
	app.Flag("retry", "retry count").Default("3").Int()
	app.Flag("verbose", "verbose").Short('v').Bool()
	app.Flag("tags", "tags").Default("a", "b", "c").Strings()

	cmd := Command(app)

	tests := []struct {
		name       string
		longhand   string
		wantDef    string
		wantValue  bool
		wantHidden bool
	}{
		{"string default", "--output=", "/tmp/out.txt", true, false},
		{"int default", "--retry=", "3", true, false},
		{"bool no default", "-v, --verbose", "", false, false},
		{"multi-value default joined", "--tags=", "a,b,c", true, false},
	}

	persistent := cmd.PersistentFlags
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, ok := persistent[tt.longhand]
			if !ok {
				t.Fatalf("persistent flag %q not found; have: %v", tt.longhand, keys(persistent))
			}
			if f.Default != tt.wantDef {
				t.Errorf("Default = %q, want %q", f.Default, tt.wantDef)
			}
			if f.Value != tt.wantValue {
				t.Errorf("Value = %v, want %v", f.Value, tt.wantValue)
			}
			if f.Hidden != tt.wantHidden {
				t.Errorf("Hidden = %v, want %v", f.Hidden, tt.wantHidden)
			}
		})
	}
}

func TestCommandBoolNegationDefaultCleared(t *testing.T) {
	app := kingpin.New("app", "test app")
	app.Flag("verbose", "verbose").Default("true").Bool()

	cmd := Command(app)
	persistent := cmd.PersistentFlags

	// original bool flag keeps its default (no shorthand, so key is just the longhand)
	if f, ok := persistent["--verbose"]; !ok {
		t.Fatal("original bool flag not found")
	} else if f.Default != "true" {
		t.Errorf("original bool Default = %q, want %q", f.Default, "true")
	}

	// --no- negation has default cleared and is hidden
	if f, ok := persistent["--no-verbose&"]; !ok {
		t.Fatal("--no-verbose negation not found")
	} else {
		if f.Default != "" {
			t.Errorf("negation Default = %q, want empty", f.Default)
		}
		if !f.Hidden {
			t.Error("negation should be hidden")
		}
	}
}

func TestCommandPersistentFlagsOnRoot(t *testing.T) {
	app := kingpin.New("app", "test app")
	app.Flag("global", "global flag").String()
	sub := app.Command("run", "run something")
	sub.Flag("local", "local flag").String()

	cmd := Command(app)

	if _, ok := cmd.PersistentFlags["--global="]; !ok {
		t.Error("root flag should be in PersistentFlags")
	}
	if len(cmd.Commands) != 1 {
		t.Fatalf("expected 1 subcommand, got %d", len(cmd.Commands))
	}
	runCmd := cmd.Commands[0]
	if _, ok := runCmd.PersistentFlags["--local="]; ok {
		t.Error("subcommand flag should not be in PersistentFlags")
	}
	if _, ok := runCmd.Flags["--local="]; !ok {
		t.Error("subcommand flag should be in subcommand Flags")
	}
}

func keys(fs command.FlagSet) []string {
	out := make([]string, 0, len(fs))
	for k := range fs {
		out = append(out, k)
	}
	return out
}
