package edge

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ongridio/ongrid/internal/pkg/errs"
	"github.com/ongridio/ongrid/internal/pkg/tunnel"
)

const databaseMetricsSecretDir = "/var/lib/ongrid-edge/secrets"

var databaseMetricsSourceIDRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

func (uc *PluginConfigUC) prepareDatabaseMetricsSpec(ctx context.Context, edgeID uint64, spec map[string]interface{}) (map[string]interface{}, error) {
	if spec == nil {
		return map[string]interface{}{}, nil
	}
	rawSources, ok := spec["sources"]
	if !ok {
		return spec, nil
	}
	sources, ok := rawSources.([]interface{})
	if !ok {
		return nil, fmt.Errorf("%w: databasemetrics.sources must be an array", errs.ErrInvalid)
	}
	nextSources := make([]interface{}, 0, len(sources))
	for i, raw := range sources {
		source, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("%w: databasemetrics.sources[%d] must be an object", errs.ErrInvalid, i)
		}
		nextSource, secretReq, err := sanitizeDatabaseMetricsSource(i, source)
		if err != nil {
			return nil, err
		}
		if secretReq != nil {
			if uc.secretWriter == nil {
				return nil, fmt.Errorf("databasemetrics secret writer is not configured")
			}
			writeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			err := uc.secretWriter.WriteDatabaseMetricsSecret(writeCtx, edgeID, *secretReq)
			cancel()
			if err != nil {
				return nil, fmt.Errorf("write databasemetrics secret source %q: %w", secretReq.SourceID, err)
			}
		}
		nextSources = append(nextSources, nextSource)
	}
	out := make(map[string]interface{}, len(spec))
	for k, v := range spec {
		out[k] = v
	}
	out["sources"] = nextSources
	return out, nil
}

func sanitizeDatabaseMetricsSource(i int, source map[string]interface{}) (map[string]interface{}, *tunnel.WriteDatabaseMetricsSecretRequest, error) {
	id := strings.TrimSpace(mapString(source, "id"))
	if !databaseMetricsSourceIDRE.MatchString(id) {
		return nil, nil, fmt.Errorf("%w: databasemetrics.sources[%d].id must match %s", errs.ErrInvalid, i, databaseMetricsSourceIDRE.String())
	}
	dbType := strings.ToLower(strings.TrimSpace(mapString(source, "db_type")))
	if !databaseMetricsDBTypeSupported(dbType) {
		return nil, nil, fmt.Errorf("%w: databasemetrics.sources[%d].db_type unsupported %q", errs.ErrInvalid, i, dbType)
	}
	secretPath := databaseMetricsSecretPath(id, dbType)
	out := make(map[string]interface{}, len(source)+1)
	for k, v := range source {
		if k == "credentials" {
			continue
		}
		out[k] = v
	}
	out["connection"] = map[string]interface{}{
		"type":       "managed",
		"path":       secretPath,
		"secret_set": true,
	}
	credentials, hasCredentials := mapValue(source["credentials"])
	if !hasCredentials {
		if connectionSecretSet(source) {
			return out, nil, nil
		}
		return nil, nil, fmt.Errorf("%w: databasemetrics.sources[%d].credentials required", errs.ErrInvalid, i)
	}
	content, err := buildDatabaseMetricsSecret(dbType, credentials)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: databasemetrics.sources[%d].credentials: %v", errs.ErrInvalid, i, err)
	}
	return out, &tunnel.WriteDatabaseMetricsSecretRequest{
		SourceID: id,
		Path:     secretPath,
		Content:  content,
	}, nil
}

func connectionSecretSet(source map[string]interface{}) bool {
	connection, ok := mapValue(source["connection"])
	if !ok {
		return false
	}
	v, _ := connection["secret_set"].(bool)
	return v
}

func databaseMetricsSecretPath(id, dbType string) string {
	ext := ".dsn"
	if dbType == "mysql" {
		ext = ".my.cnf"
	}
	return filepath.Join(databaseMetricsSecretDir, id+ext)
}

func databaseMetricsDBTypeSupported(v string) bool {
	switch v {
	case "mysql", "postgresql", "redis", "mongodb":
		return true
	default:
		return false
	}
}

func buildDatabaseMetricsSecret(dbType string, credentials map[string]interface{}) (string, error) {
	c := dbCredentials{
		Host:       mapStringDefault(credentials, "host", "127.0.0.1"),
		Port:       mapString(credentials, "port"),
		Username:   mapString(credentials, "username"),
		Password:   mapString(credentials, "password"),
		Database:   mapString(credentials, "database"),
		SSLMode:    mapString(credentials, "sslmode"),
		AuthSource: mapString(credentials, "auth_source"),
	}
	if err := c.validate(); err != nil {
		return "", err
	}
	switch dbType {
	case "mysql":
		if c.Port == "" {
			c.Port = "3306"
		}
		return buildMySQLSecret(c), nil
	case "postgresql":
		if c.Port == "" {
			c.Port = "5432"
		}
		if c.Database == "" {
			c.Database = "postgres"
		}
		if c.SSLMode == "" {
			c.SSLMode = "disable"
		}
		return buildPostgresDSN(c), nil
	case "redis":
		if c.Port == "" {
			c.Port = "6379"
		}
		if c.Database == "" {
			c.Database = "0"
		}
		if _, err := strconv.Atoi(c.Database); err != nil {
			return "", fmt.Errorf("database must be a Redis DB index")
		}
		return buildRedisURI(c), nil
	case "mongodb":
		if c.Port == "" {
			c.Port = "27017"
		}
		if c.Database == "" {
			c.Database = "admin"
		}
		if c.AuthSource == "" {
			c.AuthSource = c.Database
		}
		return buildMongoURI(c), nil
	default:
		return "", fmt.Errorf("unsupported db_type %q", dbType)
	}
}

type dbCredentials struct {
	Host       string
	Port       string
	Username   string
	Password   string
	Database   string
	SSLMode    string
	AuthSource string
}

func (c dbCredentials) validate() error {
	for name, value := range map[string]string{
		"host":        c.Host,
		"port":        c.Port,
		"username":    c.Username,
		"password":    c.Password,
		"database":    c.Database,
		"sslmode":     c.SSLMode,
		"auth_source": c.AuthSource,
	} {
		if strings.ContainsAny(value, "\r\n") {
			return fmt.Errorf("%s must not contain newlines", name)
		}
	}
	if strings.TrimSpace(c.Host) == "" {
		return fmt.Errorf("host required")
	}
	if c.Port != "" {
		n, err := strconv.Atoi(c.Port)
		if err != nil || n <= 0 || n > 65535 {
			return fmt.Errorf("port must be 1..65535")
		}
	}
	return nil
}

func buildMySQLSecret(c dbCredentials) string {
	lines := []string{"[client]"}
	if c.Username != "" {
		lines = append(lines, "user="+c.Username)
	}
	if c.Password != "" {
		lines = append(lines, "password="+c.Password)
	}
	lines = append(lines, "host="+c.Host)
	if c.Port != "" {
		lines = append(lines, "port="+c.Port)
	}
	if c.Database != "" {
		lines = append(lines, "database="+c.Database)
	}
	return strings.Join(lines, "\n")
}

func buildPostgresDSN(c dbCredentials) string {
	u := url.URL{
		Scheme: "postgresql",
		Host:   net.JoinHostPort(c.Host, c.Port),
		Path:   "/" + c.Database,
	}
	setUserInfo(&u, c)
	q := u.Query()
	q.Set("sslmode", c.SSLMode)
	u.RawQuery = q.Encode()
	return u.String()
}

func buildRedisURI(c dbCredentials) string {
	u := url.URL{
		Scheme: "redis",
		Host:   net.JoinHostPort(c.Host, c.Port),
		Path:   "/" + c.Database,
	}
	setUserInfo(&u, c)
	return u.String()
}

func buildMongoURI(c dbCredentials) string {
	u := url.URL{
		Scheme: "mongodb",
		Host:   net.JoinHostPort(c.Host, c.Port),
		Path:   "/" + c.Database,
	}
	setUserInfo(&u, c)
	if c.AuthSource != "" {
		q := u.Query()
		q.Set("authSource", c.AuthSource)
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func setUserInfo(u *url.URL, c dbCredentials) {
	if c.Username == "" && c.Password == "" {
		return
	}
	if c.Password == "" {
		u.User = url.User(c.Username)
		return
	}
	u.User = url.UserPassword(c.Username, c.Password)
}

func mapValue(v interface{}) (map[string]interface{}, bool) {
	m, ok := v.(map[string]interface{})
	return m, ok
}

func mapString(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return strings.TrimSpace(v)
}

func mapStringDefault(m map[string]interface{}, key, def string) string {
	v := mapString(m, key)
	if v == "" {
		return def
	}
	return v
}
