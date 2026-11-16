// sailor-go
// Copyright (C) 2025 SailorHQ and Ashish Shekar (codekidX)

// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.
package opts

import (
	"net/http"
	"time"
)

type FetchOption int

const (
	VOLUME FetchOption = iota + 1
	PULL
	DEV
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// Logger receives structured events. Event names are part of the consumer
// contract (SAILOR_CONSUMER.md section 11) and kv is alternating key/value.
type Logger interface {
	Log(level Level, event string, kv ...any)
}

// DecodeFunc unmarshals raw bytes into target.
type DecodeFunc func(raw []byte, target any) error

type ConnectionOption struct {
	URI       string
	Addr      string
	Namespace string
	App       string
	AccessKey string
	SecretKey string
	Token     string
	Env       string
}

type InitOption struct {
	Connection *ConnectionOption

	// Resources declares every resource the consumer manages. At least one is required.
	Resources []ResourceOption

	// Watch toggles file watching for VOLUME and DEV resources. Nil means true.
	Watch *bool

	// UseLocalConfig reads the connection from ~/.sailor/config. Namespace and
	// App must still be set in Connection.
	UseLocalConfig bool

	// RequestTimeout bounds each resource fetch. Zero means 10s.
	RequestTimeout time.Duration

	// ReportTimeout bounds each consumer-report POST. Zero means 5s.
	ReportTimeout time.Duration

	// Logger receives events. Nil means a structured logger on stderr.
	Logger Logger

	// HTTPClient performs every request. Its own Timeout is ignored in favour
	// of RequestTimeout and ReportTimeout. Nil means a default client.
	HTTPClient *http.Client

	// Decoders adds or replaces decoders for a lang, keyed by normalised lang.
	// Decoders are per consumer instance.
	Decoders map[string]DecodeFunc
}

type ResourceDefinition struct {
	Name      string
	Lang      string
	Encrypted bool
	Path      string
}

type FetchDefinition struct {
	Fetch        FetchOption
	Once         bool
	PullInterval time.Duration
}

type ResourceOption struct {
	Def      ResourceDefinition
	FetchDef FetchDefinition

	// Decode turns resolved bytes into the stored value. Nil stores the bytes
	// verbatim. decode is the consumer's decoder for the resource's lang.
	Decode func(raw []byte, decode DecodeFunc) (any, error)
}
