package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestRunUsageAndVersion(t *testing.T) {
	var out, errOut bytes.Buffer
	env := func(string) string { return "" }

	if code := run(context.Background(), nil, env, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "Usage") {
		t.Fatalf("no command: code=%d stderr=%q", code, errOut.String())
	}
	out.Reset()
	if code := run(context.Background(), []string{"version"}, env, &out, &errOut); code != 0 || !strings.HasPrefix(out.String(), "dev") {
		t.Fatalf("version: code=%d out=%q", code, out.String())
	}
}

func TestRunRejectsMissingConfig(t *testing.T) {
	var out, errOut bytes.Buffer
	code := run(context.Background(), []string{"all"}, func(string) string { return "" }, &out, &errOut)
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	for _, want := range []string{"NITHRO_DISCORD_TOKEN", "NITHRO_DISCORD_CLIENT_ID", "dev-setup.md"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("error output missing %q:\n%s", want, errOut.String())
		}
	}
}
