// Command authfixture signs a trusted-proxy identity assertion for
// qualification/test use, using the exact production HMAC/canonicalization
// contract in internal/authn — it never reimplements it.
//
// TEST FIXTURE. NOT A PRODUCTION PROXY. It exists solely so
// hack/ui-lima-auth.sh and manual qualification checks can construct real,
// validly-signed (or deliberately stale) requests without hand-rolling the
// signature. It never reads a secret from anywhere except the file named by
// -secret-file, never logs or prints the secret bytes, and grants no
// authority beyond producing header values a caller must still send.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/authn"
)

func main() {
	secretFile := flag.String("secret-file", "", "path to the raw HMAC secret bytes (required)")
	user := flag.String("user", "", "username to assert (required)")
	groups := flag.String("groups", "", "comma-separated groups to assert")
	age := flag.Duration("age", 0, "how far in the past to backdate the assertion's timestamp (0 = now); use a value beyond the server's freshness window to produce a deliberately stale assertion")
	format := flag.String("format", "curl", "output format: curl (repeatable -H flags) or env (shell-quoted VAR=value lines)")
	flag.Parse()

	if *secretFile == "" || *user == "" {
		fmt.Fprintln(os.Stderr, "authfixture: -secret-file and -user are required")
		os.Exit(2)
	}
	secret, err := os.ReadFile(*secretFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "authfixture: read secret: %v\n", err)
		os.Exit(1)
	}

	var groupList []string
	if *groups != "" {
		groupList = strings.Split(*groups, ",")
	}
	timestamp := time.Now().Add(-*age)
	headers, err := authn.SignAssertionHeaders(secret, authn.Identity{Username: *user, Groups: groupList}, timestamp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "authfixture: sign: %v\n", err)
		os.Exit(1)
	}

	switch *format {
	case "curl":
		for _, name := range []string{authn.UserHeader, authn.GroupsHeader, authn.ProxyHeader, authn.TimestampHeader, authn.SignatureHeader} {
			fmt.Printf("-H %q\n", fmt.Sprintf("%s: %s", name, headers.Get(name)))
		}
	case "env":
		for _, name := range []string{authn.UserHeader, authn.GroupsHeader, authn.ProxyHeader, authn.TimestampHeader, authn.SignatureHeader} {
			fmt.Printf("%s=%s\n", envName(name), headers.Get(name))
		}
	default:
		fmt.Fprintf(os.Stderr, "authfixture: unknown -format %q\n", *format)
		os.Exit(2)
	}
}

func envName(header string) string {
	return strings.ToUpper(strings.ReplaceAll(header, "-", "_"))
}
