package config

import (
	"errors"
	"flag"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// envOf returns a getenv that reads from a map.
func envOf(m map[string]string) func(string) string {
	return func(key string) string { return m[key] }
}

func TestDefaults(t *testing.T) {
	cfg, warnings, err := Load(nil, envOf(nil), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		HTTPAddr: "localhost:8080", GRPCAddr: "localhost:9090", PprofAddr: "localhost:6060",
		BannedWordsPath:   "banned_words.txt",
		RequestsPerSecond: 20, PostsPerSecond: 20, SignUpsPerMinute: 10, LoginsPerMinute: 10,
		LogFormat: "text", LogLevel: slog.LevelInfo,
	}
	if cfg != want {
		t.Errorf("defaults =\n %+v\nwant\n %+v", cfg, want)
	}
	if len(warnings) != 2 || !strings.Contains(warnings[0], "admin API off") || !strings.Contains(warnings[1], "game API off") {
		t.Errorf("warnings = %q, want two saying the admin and game APIs are off", warnings)
	}
}

// A flag beats an environment variable, which beats the default.
func TestPrecedence(t *testing.T) {
	env := envOf(map[string]string{
		"CHAT_HTTP_ADDR":          ":8080",
		"CHAT_GRPC_ADDR":          ":9999",
		"CHAT_SIGNUPS_PER_MINUTE": "0",
		"CHAT_LOG_FORMAT":         "JSON",
		"CHAT_ADMIN_TOKEN":        "a-long-enough-admin-token",
		"CHAT_GAME_KEY":           "a-long-enough-game-key",
	})
	cfg, warnings, err := Load([]string{"-grpc-addr", "127.0.0.1:7000", "-log-level", "debug"}, env, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HTTPAddr != ":8080" {
		t.Errorf("HTTPAddr = %q, want the env value :8080", cfg.HTTPAddr)
	}
	if cfg.GRPCAddr != "127.0.0.1:7000" {
		t.Errorf("GRPCAddr = %q, want the flag value, not the env one", cfg.GRPCAddr)
	}
	if cfg.SignUpsPerMinute != 0 || cfg.LogFormat != "json" || cfg.LogLevel != slog.LevelDebug {
		t.Errorf("got %+v", cfg)
	}
	if cfg.AdminToken != "a-long-enough-admin-token" || cfg.GameKey != "a-long-enough-game-key" || len(warnings) != 0 {
		t.Errorf("AdminToken = %q, GameKey = %q, warnings = %q; want both secrets and no warnings", cfg.AdminToken, cfg.GameKey, warnings)
	}
}

func TestShortSecretsAreRefused(t *testing.T) {
	cfg, warnings, err := Load(nil, envOf(map[string]string{"CHAT_ADMIN_TOKEN": "short", "CHAT_GAME_KEY": "short"}), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AdminToken != "" || cfg.GameKey != "" {
		t.Error("a short admin token or game key was accepted")
	}
	if len(warnings) != 2 || !strings.Contains(warnings[0], "at least 16") || !strings.Contains(warnings[1], "at least 16") {
		t.Errorf("warnings = %q, want two about the length", warnings)
	}
}

// Every bad value is reported at once.
func TestBadValues(t *testing.T) {
	_, _, err := Load([]string{"-posts-per-second", "fast", "-signups-per-minute", "-1", "-log-format", "xml", "-log-level", "loud"}, envOf(nil), io.Discard)
	if err == nil {
		t.Fatal("bad values were accepted")
	}
	for _, want := range []string{"posts-per-second", "signups-per-minute", "log-format", "log-level"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q doesn't mention %s", err, want)
		}
	}

	if _, _, err := Load([]string{"extra"}, envOf(nil), io.Discard); err == nil {
		t.Error("a stray argument was accepted")
	}
	if _, _, err := Load([]string{"-h"}, envOf(nil), io.Discard); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h = %v, want flag.ErrHelp", err)
	}
}
