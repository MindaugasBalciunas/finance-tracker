package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"ft/internal/mcptools"
	"ft/internal/oauth"
)

// The remote MCP endpoint (/mcp) and the OAuth sign-in it needs, so an AI
// app — a custom connector in Claude chat, Claude Code, any MCP client —
// reaches the same tools as the stdio server, with the owner's consent.
//
// These paths sit outside /api: nginx lets them through without the web
// password (the AI app's servers can't type it), and each checks its own
// credentials — a connection's access token for /mcp, the OAuth exchange
// for /oauth/*. Approving a connection is an ordinary session-only API call
// behind the app lock.
func (s *Server) mcpRoutes() {
	prm := func(w http.ResponseWriter, r *http.Request) (any, error) {
		b := s.publicBase(r)
		return map[string]any{"resource": b + "/mcp", "authorization_servers": []string{b}, "scopes_supported": []string{"read", "write"},
			"bearer_methods_supported": []string{"header"}, "resource_name": "Finance Tracker"}, nil
	}
	s.handle("GET /.well-known/oauth-protected-resource", prm)
	s.handle("GET /.well-known/oauth-protected-resource/mcp", prm)
	s.handle("GET /.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) (any, error) {
		b := s.publicBase(r)
		return map[string]any{"issuer": b, "authorization_endpoint": b + "/oauth/authorize", "token_endpoint": b + "/oauth/token",
			"registration_endpoint": b + "/oauth/register", "response_types_supported": []string{"code"},
			"grant_types_supported": []string{"authorization_code", "refresh_token"}, "code_challenge_methods_supported": []string{"S256"},
			"token_endpoint_auth_methods_supported": []string{"none"}, "scopes_supported": []string{"read", "write"}}, nil
	})
	s.mux.HandleFunc("POST /oauth/register", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name      string   `json:"client_name"`
			Redirects []string `json:"redirect_uris"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			oauthErr(w, &oauth.Error{Code: "invalid_client_metadata", Desc: "invalid JSON"})
			return
		}
		c, err := oauth.Register(s.DB, in.Name, in.Redirects)
		if err != nil {
			oauthErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{"client_id": c.ID, "client_name": c.Name, "redirect_uris": c.RedirectURIs,
			"token_endpoint_auth_method": "none", "grant_types": []string{"authorization_code", "refresh_token"}, "response_types": []string{"code"}})
	})
	// The consent page is the app's own (#/connect, behind its password and
	// lock); this only hands the request over to it. The web app uses
	// relative asset paths, so it can't be served from /oauth/ itself.
	s.mux.HandleFunc("GET /oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, "/#/connect?"+r.URL.RawQuery, http.StatusFound)
	})
	s.mux.HandleFunc("POST /oauth/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			oauthErr(w, &oauth.Error{Code: "invalid_request", Desc: "form body expected"})
			return
		}
		f := r.PostForm
		var t *oauth.Tokens
		var err error
		switch f.Get("grant_type") {
		case "authorization_code":
			t, err = s.OAuth.Exchange(f.Get("client_id"), f.Get("code"), f.Get("code_verifier"), f.Get("redirect_uri"))
		case "refresh_token":
			t, err = s.OAuth.Refresh(f.Get("client_id"), f.Get("refresh_token"))
		default:
			err = &oauth.Error{Code: "unsupported_grant_type", Desc: "authorization_code or refresh_token"}
		}
		if err != nil {
			oauthErr(w, err)
			return
		}
		w.Header().Set("Pragma", "no-cache")
		writeJSON(w, http.StatusOK, t)
	})

	// The consent page (web, /oauth/authorize) checks the request, then
	// sends the owner's answer.
	s.handle("GET /api/oauth/request", func(w http.ResponseWriter, r *http.Request) (any, error) {
		req := authRequest(r.URL.Query())
		c, err := oauth.Check(s.DB, req)
		if err != nil {
			return nil, bad(oauthMsg(err))
		}
		u, _ := url.Parse(req.RedirectURI)
		return map[string]any{"client_name": c.Name, "redirect_host": u.Host}, nil
	})
	s.handle("POST /api/oauth/approve", func(w http.ResponseWriter, r *http.Request) (any, error) {
		var in struct {
			oauth.Request
			Allow bool   `json:"allow"`
			Scope string `json:"scope"`
		}
		if err := decode(r, &in); err != nil {
			return nil, err
		}
		var to string
		var err error
		if in.Allow {
			to, err = s.OAuth.Approve(in.Request, in.Scope)
		} else {
			to, err = oauth.Deny(s.DB, in.Request)
		}
		if err != nil {
			return nil, bad(oauthMsg(err))
		}
		return map[string]string{"redirect": to}, nil
	})
	s.handle("GET /api/oauth/grants", func(w http.ResponseWriter, r *http.Request) (any, error) {
		g, err := oauth.Grants(s.DB)
		return map[string]any{"grants": g, "mcp_url": s.publicBase(r) + "/mcp", "public_url_set": s.PublicURL != ""}, err
	})
	s.handle("DELETE /api/oauth/grants/{id}", func(w http.ResponseWriter, r *http.Request) (any, error) {
		id, err := idParam(r, "id")
		if err != nil {
			return nil, err
		}
		return map[string]bool{"ok": true}, oauth.Revoke(s.DB, id)
	})

	h := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		scope, _ := r.Context().Value(mcpScopeKey{}).(string)
		srv := mcptools.New(s.localCaller(scope))
		if scope != oauth.ReadWrite {
			srv.RemoveTools(mcptools.WriteTools...)
		}
		return srv
	}, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true,
		// Requests reach the app from nginx on 127.0.0.1 carrying the public
		// host name; the DNS-rebinding guard would refuse every one. Every
		// request here carries a bearer token instead.
		DisableLocalhostProtection: true,
	})
	s.mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		tok = strings.TrimSpace(tok)
		// Only a connection's own token: /mcp skips the web password, and the
		// plain API tokens (ftk_/ftkw_) were never meant to work without it.
		scope := s.OAuth.Scope(tok)
		if scope == "" {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Bearer resource_metadata="%s/.well-known/oauth-protected-resource"`, s.publicBase(r)))
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid_token", "error_description": "connect the app to Finance Tracker first"})
			return
		}
		h.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), mcpScopeKey{}, scope)))
	})
}

type mcpScopeKey struct{}

func authRequest(q url.Values) oauth.Request {
	return oauth.Request{ClientID: q.Get("client_id"), RedirectURI: q.Get("redirect_uri"), State: q.Get("state"),
		CodeChallenge: q.Get("code_challenge"), Method: q.Get("code_challenge_method"), ResponseType: q.Get("response_type")}
}

func oauthMsg(err error) string {
	var oe *oauth.Error
	if errors.As(err, &oe) {
		return oe.Desc
	}
	return err.Error()
}

func oauthErr(w http.ResponseWriter, err error) {
	var oe *oauth.Error
	if !errors.As(err, &oe) {
		log.Printf("oauth: %v", err)
		oe = &oauth.Error{Code: "server_error", Desc: "internal error"}
	}
	status := http.StatusBadRequest
	if oe.Code == "invalid_client" {
		status = http.StatusUnauthorized
	}
	writeJSON(w, status, oe)
}

// publicBase is the app's address as AI apps reach it: the add-on's
// public_url option, or else the request's host (https unless it is a local
// name or address — Claude only connects over https anyway).
func (s *Server) publicBase(r *http.Request) string {
	if s.PublicURL != "" {
		return s.PublicURL
	}
	host := r.Host // not X-Forwarded-Host: nginx passes it through from the client
	proto := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-Proto"), ",")[0])
	if proto != "http" && proto != "https" {
		proto = "https"
		h := host
		if hh, _, err := net.SplitHostPort(host); err == nil {
			h = hh
		}
		if r.TLS == nil && (h == "localhost" || net.ParseIP(h) != nil || strings.HasSuffix(h, ".local")) {
			proto = "http"
		}
	}
	return proto + "://" + host
}

// PublicURLFromEnv reads FT_PUBLIC_URL (set by run.sh from the add-on's
// public_url option), trimmed of a trailing slash.
func PublicURLFromEnv() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv("FT_PUBLIC_URL")), "/")
}

// localCaller runs the MCP tools' API calls in process, as the connection:
// the same token scope as a bearer token (checked here too, so it holds
// even when the app lock is off), then the routes themselves.
func (s *Server) localCaller(scope string) mcptools.Caller {
	return func(method, path string, q url.Values, body any) (out []byte, err error) {
		p := "/api" + path
		if !tokenAllows(scope, method, p) {
			if scope != oauth.ReadWrite && method != http.MethodGet {
				return nil, errors.New("this connection is read-only — connect again with read and write to change data")
			}
			return nil, errors.New("outside this connection's scope")
		}
		var rd io.Reader
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				return nil, err
			}
			rd = bytes.NewReader(b)
		}
		u := p
		if len(q) > 0 {
			u += "?" + q.Encode()
		}
		req, err := http.NewRequest(method, u, rd)
		if err != nil {
			return nil, err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		defer func() {
			if v := recover(); v != nil {
				log.Printf("panic in MCP call %s %s: %v", method, p, v)
				out, err = nil, errors.New("internal error")
			}
		}()
		rec := httptest.NewRecorder()
		s.dispatch(rec, req)
		res := rec.Body.Bytes()
		if rec.Code >= 300 {
			var e struct {
				Error string `json:"error"`
			}
			if json.Unmarshal(res, &e) == nil && e.Error != "" {
				return nil, errors.New(e.Error)
			}
			return nil, fmt.Errorf("API returned %d", rec.Code)
		}
		return res, nil
	}
}
