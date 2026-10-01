package openbanking

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testKeyPEM mints a throwaway RSA key. Never a fixture: a committed private
// key is a private key, even a test one.
func testKeyPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
}

// pointAt swaps BaseURL to a test server for the duration of the test.
func pointAt(t *testing.T, url string) {
	t.Helper()
	orig := BaseURL
	BaseURL = url
	t.Cleanup(func() { BaseURL = orig })
}

// newTestClient builds a client aimed at srvURL.
func newTestClient(t *testing.T, appID, srvURL string) *Client {
	t.Helper()
	pointAt(t, srvURL)
	c, err := New(appID, testKeyPEM(t))
	require.NoError(t, err)
	return c
}

// kidOf pulls the `kid` out of a bearer token without verifying it — the test
// is asserting what we *sent*, not whether the provider would accept it.
func kidOf(t *testing.T, authHeader string) string {
	t.Helper()
	require.True(t, strings.HasPrefix(authHeader, "Bearer "), "Authorization = %q", authHeader)
	tok, _, err := jwt.NewParser().ParseUnverified(strings.TrimPrefix(authHeader, "Bearer "), jwt.MapClaims{})
	require.NoError(t, err)
	kid, _ := tok.Header["kid"].(string)
	return kid
}

func TestClientASPSPs(t *testing.T) {
	const appID = "11111111-2222-3333-4444-555555555555"
	var gotAuth, gotPath, gotCountry string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotCountry = r.URL.Query().Get("country")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"aspsps":[
			{"name":"Swedbank","country":"LT","bic":"HABALT22","beta":false,"sandbox":false,
			 "psu_types":["personal"],"maximum_consent_validity":15552000,
			 "required_psu_headers":["Psu-Ip-Address","Psu-User-Agent"]},
			{"name":"Revolut","country":"LT","maximum_consent_validity":7776000}
		]}`))
	}))
	defer srv.Close()
	c := newTestClient(t, appID, srv.URL)

	banks, err := c.ASPSPs(context.Background(), "LT")
	require.NoError(t, err)

	assert.Equal(t, "/aspsps", gotPath)
	assert.Equal(t, "LT", gotCountry)
	// The application id travels as the JWT `kid`, not as a claim or a header
	// of its own — get this wrong and every call is a 401 with no hint why.
	assert.Equal(t, appID, kidOf(t, gotAuth))

	// The provider wraps the list in an {"aspsps":[...]} envelope; the client
	// unwraps it so callers never see the envelope type.
	require.Len(t, banks, 2)
	assert.Equal(t, "Swedbank", banks[0].Name)
	assert.Equal(t, "LT", banks[0].Country)
	assert.Equal(t, 15552000, banks[0].MaximumConsentValidity)
	assert.Equal(t, []string{"Psu-Ip-Address", "Psu-User-Agent"}, banks[0].RequiredPSUHeaders)
	assert.Equal(t, "Revolut", banks[1].Name)
	assert.Empty(t, banks[1].RequiredPSUHeaders)
}

func TestClientExpiredSessionIsNotRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"code":"EXPIRED_SESSION"}`))
	}))
	defer srv.Close()
	c := newTestClient(t, "app", srv.URL)

	_, err := c.Transactions(context.Background(), TxQuery{AccountUID: "acc-1"})

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusUnauthorized, apiErr.Status)
	assert.Equal(t, CodeExpiredSession, apiErr.Code)
	// The whole point: an expired consent arrives as a 401, which looks exactly
	// like bad application credentials. Branching on the code keeps a dead
	// consent from being read as a dead application (and vice versa).
	assert.True(t, apiErr.Expired(), "401 + EXPIRED_SESSION must be a re-link, not a credentials failure")
	assert.False(t, apiErr.RateLimited())
}

func TestClientRateLimited(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"code":"RATE_LIMIT_EXCEEDED","message":"too many requests"}`))
	}))
	defer srv.Close()
	c := newTestClient(t, "app", srv.URL)

	_, err := c.Transactions(context.Background(), TxQuery{AccountUID: "acc-1"})

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.True(t, apiErr.RateLimited())
	// Backing off is the fix here; re-linking the consent is not. Keeping these
	// two apart is what stops a throttled sync from nagging the user to
	// reauthorise a perfectly healthy connection.
	assert.False(t, apiErr.Expired())
}

func TestClientErrorBodyNotReflected(t *testing.T) {
	// A provider error body that reads like a leaked stack trace: an internal
	// hostname, a credential, and a marker parked past the 300-char cut.
	const tailMarker = "TAIL-SECRET-MARKER"
	detail := "upstream internal-db.corp.local refused: token=supersecretvalue" +
		strings.Repeat("x", 300) + tailMarker

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = fmt.Fprintf(w, `{"detail":%q}`, detail)
	}))
	defer srv.Close()
	c := newTestClient(t, "app", srv.URL)

	_, err := c.ASPSPs(context.Background(), "LT")
	require.Error(t, err)

	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadGateway, apiErr.Status)
	// Truncation is what keeps an unbounded upstream body out of our logs and
	// out of the UI: the body is summarised, never echoed whole.
	assert.NotContains(t, err.Error(), detail, "upstream body must not be reflected verbatim")
	assert.NotContains(t, err.Error(), tailMarker, "everything past 300 chars must be cut")
	assert.True(t, strings.HasSuffix(apiErr.Message, "…"), "message = %q", apiErr.Message)
	assert.Len(t, apiErr.Message, 300+len("…"))

	// A transport failure is the other leak path: net/http decorates its errors
	// with the full URL, and our paths carry account uids.
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	pointAt(t, deadURL)

	_, err = c.Transactions(context.Background(), TxQuery{AccountUID: "secret-account-uid"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), deadURL, "host must not reach the error string")
	assert.NotContains(t, err.Error(), "secret-account-uid", "account uid must not reach the error string")
	assert.Contains(t, err.Error(), "GET", "the operation is all the caller gets, and all it needs")
}

func TestClientTransactionsPagination(t *testing.T) {
	// Keyed by the continuation_key the client sends back, so the walk only
	// advances if the key was actually forwarded.
	pages := map[string]string{
		"": `{"transactions":[{"entry_reference":"tx-1"},{"entry_reference":"tx-2"}],"continuation_key":"key-2"}`,
		"key-2": `{"transactions":[{"entry_reference":"tx-3"},{"entry_reference":"tx-4"}],
		           "continuation_key":"key-3"}`,
		"key-3": `{"transactions":[{"entry_reference":"tx-5"}]}`,
	}
	var seenKeys, seenFrom []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("continuation_key")
		seenKeys = append(seenKeys, key)
		seenFrom = append(seenFrom, r.URL.Query().Get("date_from"))
		body, ok := pages[key]
		if !ok {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":"UNKNOWN_CONTINUATION_KEY"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := newTestClient(t, "app", srv.URL)

	from := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	rows, err := c.AllTransactions(context.Background(), TxQuery{AccountUID: "acc-1", DateFrom: from})
	require.NoError(t, err)

	assert.Equal(t, []string{"", "key-2", "key-3"}, seenKeys, "each page must carry the previous page's key")
	// The date window has to survive the walk too — dropping it on page two
	// would silently widen the pull to whatever the bank defaults to.
	assert.Equal(t, []string{"2026-07-01", "2026-07-01", "2026-07-01"}, seenFrom)

	var refs []string
	for _, r := range rows {
		refs = append(refs, r.EntryReference)
	}
	assert.Equal(t, []string{"tx-1", "tx-2", "tx-3", "tx-4", "tx-5"}, refs)
}

func TestClientNonJSONErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("<html><head><title>503 Service Unavailable</title></head><body>nginx</body></html>"))
	}))
	defer srv.Close()
	c := newTestClient(t, "app", srv.URL)

	_, err := c.ASPSPs(context.Background(), "LT")

	// A proxy that never reached the provider still has to come back as an
	// APIError: callers switch on the status, and a JSON decode failure here
	// would look like a client bug rather than an upstream outage.
	var apiErr *APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusServiceUnavailable, apiErr.Status)
	assert.Empty(t, apiErr.Code, "an HTML page carries no code to branch on")
	assert.False(t, apiErr.Expired())
	assert.NotContains(t, err.Error(), "nginx", "the proxy's page is not the user's problem")
}

func TestPSUHeadersAllOrNothing(t *testing.T) {
	full := PSU{IPAddress: "198.51.100.7", UserAgent: "Mozilla/5.0"}

	tests := []struct {
		name     string
		psu      PSU
		required []string
		want     map[string]string
	}{
		{
			name:     "all required headers satisfiable",
			psu:      full,
			required: []string{"Psu-Ip-Address", "Psu-User-Agent"},
			want: map[string]string{
				"Psu-Ip-Address": "198.51.100.7",
				"Psu-User-Agent": "Mozilla/5.0",
			},
		},
		{
			// Banks are inconsistent about casing in required_psu_headers, but
			// the name they ask for is the name we must send back.
			name:     "casing follows the bank, matching does not",
			psu:      full,
			required: []string{"PSU-IP-Address"},
			want:     map[string]string{"PSU-IP-Address": "198.51.100.7"},
		},
		{
			name:     "missing user agent drops the whole set",
			psu:      PSU{IPAddress: "198.51.100.7"},
			required: []string{"Psu-Ip-Address", "Psu-User-Agent"},
			want:     nil,
		},
		{
			name:     "missing ip address drops the whole set",
			psu:      PSU{UserAgent: "Mozilla/5.0"},
			required: []string{"Psu-Ip-Address", "Psu-User-Agent"},
			want:     nil,
		},
		{
			// A header we have no source for is the same case as an empty one.
			name:     "unknown required header drops the whole set",
			psu:      full,
			required: []string{"Psu-Ip-Address", "Psu-Device-Id"},
			want:     nil,
		},
		{
			name:     "bank requires none",
			psu:      full,
			required: nil,
			want:     nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A partial set is a 422 PSU_HEADER_NOT_PROVIDED upstream, so the
			// unattended path (nothing at all) is strictly better than half.
			assert.Equal(t, tt.want, tt.psu.headersFor(tt.required))
		})
	}

	// And the same rule at the wire: a half-known PSU must not leave a single
	// header on the request.
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"https://bank.example/auth","authorization_id":"auth-1"}`))
	}))
	defer srv.Close()
	c := newTestClient(t, "app", srv.URL)

	_, err := c.StartAuth(context.Background(), AuthParams{
		ASPSPName:          "Swedbank",
		ASPSPCountry:       "LT",
		ValidUntil:         time.Now().Add(24 * time.Hour),
		PSU:                PSU{IPAddress: "198.51.100.7"},
		RequiredPSUHeaders: []string{"Psu-Ip-Address", "Psu-User-Agent"},
	})
	require.NoError(t, err)
	assert.Empty(t, got.Get("Psu-Ip-Address"))
	assert.Empty(t, got.Get("Psu-User-Agent"))
}

// errors.As has to reach through the client's own wrapping, not just a
// top-level *APIError — this is the shape callers actually branch on.
func TestAPIErrorUnwrapsThroughCallers(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"SESSION_NOT_FOUND"}`))
	}))
	defer srv.Close()
	c := newTestClient(t, "app", srv.URL)

	err := c.DeleteSession(context.Background(), "sess-1")
	var apiErr *APIError
	require.True(t, errors.As(err, &apiErr))
	assert.True(t, apiErr.Expired())
}

// The real /aspsps answer sends `sandbox` as an object describing the test
// users, not the documented flag — and one mismatched cosmetic field aborted
// the whole decode, so the bank picker showed "Couldn't load data" with no
// bank in it. Every shape here must leave the load-bearing fields intact.
func TestASPSPDecodesFlagsWhateverShapeTheySend(t *testing.T) {
	body := `{"aspsps":[
      {"name":"Swedbank","country":"LT","sandbox":{"users":[{"username":"19901111-1111"}]},"beta":false,
       "maximum_consent_validity":15552000,"required_psu_headers":["Psu-Ip-Address"]},
      {"name":"SEB","country":"LT","sandbox":false,"beta":true,"maximum_consent_validity":7776000},
      {"name":"Artea","country":"LT","sandbox":true,"beta":[],"maximum_consent_validity":7776000},
      {"name":"Urbo","country":"LT","sandbox":null,"beta":"true","maximum_consent_validity":7776000}
    ]}`

	var out aspspsResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if len(out.ASPSPs) != 4 {
		t.Fatalf("want 4 banks, got %d", len(out.ASPSPs))
	}

	for _, tc := range []struct {
		i             int
		name          string
		sandbox, beta bool
	}{
		{0, "Swedbank", true, false}, // object -> the detail's existence is the answer
		{1, "SEB", false, true},      // plain bools still work
		{2, "Artea", true, false},    // empty array -> false
		{3, "Urbo", false, true},     // null -> false; "true" -> true
	} {
		got := out.ASPSPs[tc.i]
		if got.Name != tc.name {
			t.Fatalf("[%d] name = %q, want %q", tc.i, got.Name, tc.name)
		}
		if bool(got.Sandbox) != tc.sandbox {
			t.Errorf("%s: sandbox = %v, want %v", tc.name, got.Sandbox, tc.sandbox)
		}
		if bool(got.Beta) != tc.beta {
			t.Errorf("%s: beta = %v, want %v", tc.name, got.Beta, tc.beta)
		}
	}

	// The whole point: the fields the sync actually depends on survived.
	if out.ASPSPs[0].MaximumConsentValidity != 15552000 {
		t.Errorf("consent validity lost: %d", out.ASPSPs[0].MaximumConsentValidity)
	}
	if len(out.ASPSPs[0].RequiredPSUHeaders) != 1 {
		t.Errorf("PSU headers lost: %v", out.ASPSPs[0].RequiredPSUHeaders)
	}
}

// A genuinely malformed flag is still an error — this is tolerance for shape,
// not a blanket "ignore anything you don't understand".
func TestASPSPStillRejectsNonsense(t *testing.T) {
	var out aspspsResponse
	err := json.Unmarshal([]byte(`{"aspsps":[{"name":"X","sandbox":12.5.3}]}`), &out)
	if err == nil {
		t.Fatal("expected a decode error for malformed JSON")
	}
}
