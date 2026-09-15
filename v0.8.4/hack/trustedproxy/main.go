// Command trustedproxy is a bounded, local, qualification-only fixture that
// exercises the production trusted-proxy header/signature contract end to
// end so hack/ui-lima-auth.sh can put a real browser session behind it.
//
// TEST FIXTURE. NOT A PRODUCTION REVERSE PROXY. It does not authenticate a
// human (there is no login step: every request it forwards is asserted as
// one fixed, operator-configured qualification identity), it has no TLS
// termination, and it must never be pointed at a real cluster's traffic. It
// exists only to prove that a real Verifier-checked request/response cycle
// (401 unsigned, 200 valid signed, 401 stale signed) is reachable end to end
// through something shaped like a proxy, and to let a browser reach the
// backend at all in production mode without the browser itself holding
// credentials or being able to set the identity headers.
//
// It faithfully reproduces the two properties a real trusted proxy must
// have for the backend's trust model to hold: it strips every inbound
// client-supplied X-Operations-Center-* header before forwarding (so a
// browser cannot inject its own identity), and it signs the assertion it
// injects using the exact production internal/authn contract (via
// internal/authn.SignAssertionHeaders), not a reimplementation.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/idriss-eliguene/landlock-genprof/internal/authn"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:8090", "address for the fixture to listen on")
	backend := flag.String("backend", "http://127.0.0.1:8080", "backend Operations Center address")
	secretFile := flag.String("secret-file", "", "path to the raw HMAC secret bytes (required)")
	user := flag.String("user", "", "qualification identity to assert for every forwarded request (required)")
	groups := flag.String("groups", "", "comma-separated groups to assert")
	flag.Parse()

	if *secretFile == "" || *user == "" {
		fmt.Fprintln(os.Stderr, "trustedproxy: -secret-file and -user are required")
		os.Exit(2)
	}
	secret, err := os.ReadFile(*secretFile)
	if err != nil {
		log.Fatalf("trustedproxy: read secret: %v", err)
	}
	var groupList []string
	if *groups != "" {
		groupList = strings.Split(*groups, ",")
	}
	identity := authn.Identity{Username: *user, Groups: groupList}

	target, err := url.Parse(*backend)
	if err != nil {
		log.Fatalf("trustedproxy: parse -backend: %v", err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(r *http.Request) {
		// Strip every client-supplied identity header first. A real
		// trusted proxy must do this before it authenticates the human by
		// its own separate means and injects its own fresh assertion;
		// forwarding any inbound X-Operations-Center-* header from the
		// client would let the client forge its own identity.
		for _, name := range []string{authn.UserHeader, authn.GroupsHeader, authn.ProxyHeader, authn.TimestampHeader, authn.SignatureHeader} {
			r.Header.Del(name)
		}
		headers, signErr := authn.SignAssertionHeaders(secret, identity, time.Now())
		if signErr != nil {
			// Director cannot return an error; leave the assertion absent
			// so the backend fails closed on the missing/invalid headers
			// rather than forwarding a broken one.
			log.Printf("trustedproxy: sign assertion: %v", signErr)
		} else {
			for name, values := range headers {
				for _, v := range values {
					r.Header.Set(name, v)
				}
			}
		}
		originalDirector(r)
	}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Printf("trustedproxy: backend error: %v", err)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "trustedproxy: backend unreachable\n")
	}

	log.Printf("trustedproxy: TEST FIXTURE, NOT A PRODUCTION PROXY: listening on %s, asserting %q, forwarding to %s", *listen, *user, *backend)
	if err := http.ListenAndServe(*listen, proxy); err != nil { // #nosec G114 -- qualification-only loopback fixture, no TLS by design
		log.Fatalf("trustedproxy: %v", err)
	}
}
