// Package config reads the server's settings from command-line flags, with
// environment variables as fallbacks, and checks them.
//
// For every setting, a flag beats an environment variable, which beats the
// default:
//
//	go run ./cmd/server -http-addr :8080          # flag
//	CHAT_HTTP_ADDR=:8080 go run ./cmd/server      # environment (bash)
//
// The secrets are the exception: the admin token and the game key are read
// only from CHAT_ADMIN_TOKEN and CHAT_GAME_KEY, never from a flag, because a
// process's flags can be seen by every user on the machine (in ps, Task
// Manager, ...).
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
)

// MinAdminTokenLen is the shortest admin token the server accepts.
const MinAdminTokenLen = 16

// Config is everything the server can be told.
type Config struct {
	HTTPAddr  string // the web app, HTTP API and live feed
	GRPCAddr  string // the gRPC API
	PprofAddr string // profiling endpoints; "" turns them off

	BannedWordsPath string // the moderation list, read at start and saved on change
	AdminToken      string // "" turns the admin API off
	GameKey         string // "" turns the game API off

	RequestsPerSecond float64 // per logged-in user; 0 = no limit
	PostsPerSecond    float64 // messages per user; 0 = no limit
	SignUpsPerMinute  int     // new accounts per IP; 0 = no limit
	LoginsPerMinute   int     // login attempts per IP; 0 = no limit

	LogFormat string     // "text" or "json"
	LogLevel  slog.Level // debug, info, warn or error
}

// Load reads the settings from args (os.Args[1:] in main) and getenv
// (os.Getenv in main), and checks them. Warnings are things worth telling
// the operator about that don't stop the server, like an admin token too
// short to use.
//
// -h and -help print the flags to out and return flag.ErrHelp.
func Load(args []string, getenv func(string) string, out io.Writer) (cfg Config, warnings []string, err error) {
	env := func(key, def string) string {
		if v := getenv(key); v != "" {
			return v
		}
		return def
	}

	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	fs.SetOutput(out)
	fs.StringVar(&cfg.HTTPAddr, "http-addr", env("CHAT_HTTP_ADDR", "localhost:8080"),
		"address for the web app and HTTP API; \":8080\" also accepts other machines (env CHAT_HTTP_ADDR)")
	fs.StringVar(&cfg.GRPCAddr, "grpc-addr", env("CHAT_GRPC_ADDR", "localhost:9090"),
		"address for the gRPC API (env CHAT_GRPC_ADDR)")
	fs.StringVar(&cfg.PprofAddr, "pprof-addr", env("CHAT_PPROF_ADDR", "localhost:6060"),
		"address for pprof profiling, \"\" to turn it off; keep it on localhost (env CHAT_PPROF_ADDR)")
	fs.StringVar(&cfg.BannedWordsPath, "banned-words", env("CHAT_BANNED_WORDS", "banned_words.txt"),
		"the banned-word list (env CHAT_BANNED_WORDS)")

	requests, posts := env("CHAT_REQUESTS_PER_SECOND", "20"), env("CHAT_POSTS_PER_SECOND", "20")
	signUps, logins := env("CHAT_SIGNUPS_PER_MINUTE", "10"), env("CHAT_LOGINS_PER_MINUTE", "10")
	fs.StringVar(&requests, "requests-per-second", requests, "requests per logged-in user per second, 0 = no limit (env CHAT_REQUESTS_PER_SECOND)")
	fs.StringVar(&posts, "posts-per-second", posts, "messages each user may post per second, 0 = no limit (env CHAT_POSTS_PER_SECOND)")
	fs.StringVar(&signUps, "signups-per-minute", signUps, "new accounts per IP per minute, 0 = no limit (env CHAT_SIGNUPS_PER_MINUTE)")
	fs.StringVar(&logins, "logins-per-minute", logins, "login attempts per IP per minute, 0 = no limit (env CHAT_LOGINS_PER_MINUTE)")

	level := env("CHAT_LOG_LEVEL", "info")
	fs.StringVar(&cfg.LogFormat, "log-format", env("CHAT_LOG_FORMAT", "text"), "log as \"text\" or \"json\" (env CHAT_LOG_FORMAT)")
	fs.StringVar(&level, "log-level", level, "debug, info, warn or error (env CHAT_LOG_LEVEL)")

	if err := fs.Parse(args); err != nil {
		return Config{}, nil, err
	}
	if fs.NArg() > 0 {
		return Config{}, nil, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}

	// Parse and check the values. Collect every problem, not just the first,
	// so one run tells the operator everything to fix.
	var errs []error
	cfg.RequestsPerSecond = parseRate("requests-per-second", requests, &errs)
	cfg.PostsPerSecond = parseRate("posts-per-second", posts, &errs)
	cfg.SignUpsPerMinute = int(parseRate("signups-per-minute", signUps, &errs))
	cfg.LoginsPerMinute = int(parseRate("logins-per-minute", logins, &errs))

	if err := cfg.LogLevel.UnmarshalText([]byte(level)); err != nil {
		errs = append(errs, fmt.Errorf("-log-level: %q is not debug, info, warn or error", level))
	}
	cfg.LogFormat = strings.ToLower(cfg.LogFormat)
	if cfg.LogFormat != "text" && cfg.LogFormat != "json" {
		errs = append(errs, fmt.Errorf("-log-format: %q is not text or json", cfg.LogFormat))
	}
	if cfg.HTTPAddr == "" || cfg.GRPCAddr == "" {
		errs = append(errs, errors.New("-http-addr and -grpc-addr can't be empty"))
	}
	if cfg.BannedWordsPath == "" {
		errs = append(errs, errors.New("-banned-words can't be empty"))
	}
	if err := errors.Join(errs...); err != nil {
		return Config{}, nil, err
	}

	cfg.AdminToken = getenv("CHAT_ADMIN_TOKEN")
	switch {
	case cfg.AdminToken == "":
		warnings = append(warnings, "admin API off (set CHAT_ADMIN_TOKEN to turn it on)")
	case len(cfg.AdminToken) < MinAdminTokenLen:
		// A short token can be guessed, even with the lockout on wrong ones.
		warnings = append(warnings, fmt.Sprintf("admin API off: CHAT_ADMIN_TOKEN must be at least %d characters", MinAdminTokenLen))
		cfg.AdminToken = ""
	}
	cfg.GameKey = getenv("CHAT_GAME_KEY")
	switch {
	case cfg.GameKey == "":
		warnings = append(warnings, "game API off (set CHAT_GAME_KEY to turn it on)")
	case len(cfg.GameKey) < MinAdminTokenLen:
		warnings = append(warnings, fmt.Sprintf("game API off: CHAT_GAME_KEY must be at least %d characters", MinAdminTokenLen))
		cfg.GameKey = ""
	}
	return cfg, warnings, nil
}

// parseRate reads a non-negative number, adding an error to errs if it isn't.
func parseRate(name, s string, errs *[]error) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || v < 0 {
		*errs = append(*errs, fmt.Errorf("-%s: %q is not a number of 0 or more", name, s))
		return 0
	}
	return v
}
