package sailor

import (
	"encoding/json"
	"net"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/sailorhq/sailor-go/pkg/opts"
)

const defaultScheme = "http"

// connection is the resolved, validated connection (SAILOR_CONSUMER.md section 2).
type connection struct {
	source    string // one of: env_uri, local_config, uri_option, fields
	addr      string // always carries a scheme
	namespace string
	app       string
	accessKey string
	secretKey string
	token     string
	env       string
}

// resolveConnection applies the precedence order in CONS-CFG-4 to CONS-CFG-8.
func resolveConnection(in opts.InitOption, d deps) (connection, error) {
	if uri := d.getenv("SAILOR_URI"); uri != "" {
		c, err := parseURI(uri)
		if err != nil {
			return connection{}, err
		}
		c.source = "env_uri"
		c.token = d.getenv("SAILOR_TOKEN")
		return c, nil
	}

	conn := in.Connection
	if in.UseLocalConfig {
		if conn == nil || conn.Namespace == "" || conn.App == "" {
			return connection{}, errf(CodeMissingNamespaceApp, "Namespace and App must be set when UseLocalConfig is true")
		}
		c, err := readLocalConfig(d)
		if err != nil {
			return connection{}, err
		}
		c.source = "local_config"
		c.namespace = conn.Namespace
		c.app = conn.App
		if conn.Token != "" {
			c.token = conn.Token
		}
		return c, nil
	}

	if conn != nil && conn.URI != "" {
		c, err := parseURI(conn.URI)
		if err != nil {
			return connection{}, err
		}
		c.source = "uri_option"
		c.token = conn.Token
		if c.token == "" {
			c.token = d.getenv("SAILOR_TOKEN")
		}
		return c, nil
	}

	if conn != nil && conn.Addr != "" {
		if conn.Namespace == "" || conn.App == "" {
			return connection{}, errf(CodeMissingNamespaceApp, "Namespace and App must be set with Addr")
		}
		addr, err := normalizeAddr(conn.Addr)
		if err != nil {
			return connection{}, err
		}
		return connection{
			source:    "fields",
			addr:      addr,
			namespace: conn.Namespace,
			app:       conn.App,
			accessKey: conn.AccessKey,
			secretKey: conn.SecretKey,
			token:     conn.Token,
			env:       conn.Env,
		}, nil
	}

	return connection{}, ErrNoConnection
}

// parseURI implements the grammar in SAILOR_CONSUMER.md section 3.1. It never
// reads environment variables; the caller decides where the token comes from.
func parseURI(raw string) (connection, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return connection{}, wrapf(CodeInvalidURI, err, "URI does not parse")
	}
	if u.Scheme != "sailor" {
		return connection{}, ErrInvalidURIPrefix
	}

	accessKey := u.User.Username()
	if accessKey == "" {
		return connection{}, ErrMissingAccessKey
	}
	secretKey, ok := u.User.Password()
	if !ok || secretKey == "" {
		return connection{}, ErrMissingSecretKey
	}

	parts := strings.Split(strings.TrimPrefix(u.Path, "/"), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return connection{}, ErrMissingPathComponents
	}

	scheme := defaultScheme
	if values, present := u.Query()["scheme"]; present {
		scheme = values[0]
		if scheme != "http" && scheme != "https" {
			return connection{}, errf(CodeInvalidScheme, "scheme parameter must be http or https")
		}
	}
	if u.Host == "" {
		return connection{}, errf(CodeNoConnection, "URI has no host")
	}

	return connection{
		addr:      scheme + "://" + u.Host,
		namespace: parts[0],
		app:       parts[1],
		accessKey: accessKey,
		secretKey: secretKey,
	}, nil
}

// normalizeAddr implements CONS-URI-10 to CONS-URI-12.
func normalizeAddr(addr string) (string, error) {
	if addr == "" {
		return "", ErrNoConnection
	}
	if strings.Contains(addr, "://") {
		return addr, nil
	}
	return defaultScheme + "://" + addr, nil
}

// isLoopbackAddr reports whether addr points at the local machine (CONS-URI-13).
func isLoopbackAddr(addr string) bool {
	u, err := url.Parse(addr)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type localConfigFile struct {
	Manifest struct {
		Envs []struct {
			Name string `json:"name"`
			Host string `json:"host"`
		} `json:"envs"`
	} `json:"manifest"`
	Env    string            `json:"env"`
	Tokens map[string]string `json:"tokens"`
}

// readLocalConfig implements SAILOR_CONSUMER.md section 12.
func readLocalConfig(d deps) (connection, error) {
	home, err := d.homeDir()
	if err != nil {
		return connection{}, wrapf(CodeLocalConfigNotFound, err, "cannot find home directory")
	}
	data, err := d.fs.ReadFile(filepath.Join(home, ".sailor", "config"))
	if err != nil {
		return connection{}, wrapf(CodeLocalConfigNotFound, err, "reading ~/.sailor/config")
	}
	var cfg localConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		return connection{}, wrapf(CodeLocalConfigInvalid, err, "parsing ~/.sailor/config")
	}
	for _, e := range cfg.Manifest.Envs {
		if e.Name == cfg.Env {
			addr, err := normalizeAddr(e.Host)
			if err != nil {
				return connection{}, err
			}
			return connection{addr: addr, token: cfg.Tokens[cfg.Env], env: cfg.Env}, nil
		}
	}
	return connection{}, errf(CodeLocalConfigEnvMissing, "active environment %q is not in the manifest", cfg.Env)
}
