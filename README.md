# 🐧 Sailor Go - Consumer

[![Go Report Card](https://goreportcard.com/badge/github.com/sailorhq/sailor-go)](https://goreportcard.com/report/github.com/sailorhq/sailor-go)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL%203.0-green.svg)](https://opensource.org/licenses/GPL-3.0)
[![Go Version](https://img.shields.io/github/go-mod/go-version/sailorhq/sailor-go)](https://golang.org)

> **A powerful Go library for seamless configuration and secret management in
> Kubernetes environments**

Sailor-Go is a robust, type-safe configuration management library designed
specifically for Go applications running in Kubernetes environments. It provides
intelligent resource management with support for ConfigMaps, Secrets, and custom
resources with automatic fallback mechanisms.

## ✨ Features

- 🔄 **Real-time Configuration Updates** - Watch for changes in mounted volumes
  and ConfigMaps
- 🔐 **Secure Secret Management** - Handle Kubernetes secrets with type safety
- 📡 **Pull-based Updates** - Fetch configurations from remote Sailor API
- 🛡️ **Fallback Support** - Automatic fallback mechanisms for high availability
- 🎯 **Type Safety** - Generic types ensure compile-time safety
- ⚡ **High Performance** - Atomic operations and efficient resource management
- 🐳 **Kubernetes Native** - Designed specifically for K8s environments

## 🚀 Quick Start

### Installation

```bash
go get github.com/sailorhq/sailor-go
```

### Basic Usage

```go
package main

import (
    "log"
    "time"
    
    "github.com/sailorhq/sailor-go"
    "github.com/sailorhq/sailor-go/pkg/opts"
)

// Define your configuration structure
type AppConfig struct {
    DatabaseURL string `json:"database_url"`
    APIKey      string `json:"api_key"`
    Port        int    `json:"port"`
}

// Define your secrets structure
type AppSecrets struct {
    DatabasePassword string `json:"db_password"`
    JWTSecret       string `json:"jwt_secret"`
}

func main() {
    // Create initialization options
    initOpts := opts.InitOption{
        Connection: &opts.ConnectionOption{
            Addr:      "https://sailor.example.com",
            Namespace: "my-app",
            App:       "web-service",
            AccessKey: "your-access-key",
            SecretKey: "your-secret-key",
        },
        Resources: []opts.ResourceOption{
            sailor.ConfigMapDefault(),    // Use default ConfigMap
            sailor.SecretsDefault(),      // Use default Secrets
        },
    }

    // Create consumer
    consumer, err := sailor.NewConsumer[AppConfig, AppSecrets](initOpts)
    if err != nil {
        log.Fatal(err)
    }

    // Start the consumer
    if err := consumer.Start(); err != nil {
        log.Fatal(err)
    }

    // Use configurations
    config, err := consumer.Get()
    if err != nil {
        log.Printf("Error getting config: %v", err)
        return
    }

    secrets, err := consumer.GetSecret()
    if err != nil {
        log.Printf("Error getting secrets: %v", err)
        return
    }

    log.Printf("Database URL: %s", config.DatabaseURL)
    log.Printf("Database Password: %s", secrets.DatabasePassword)
}
```

## 📚 Examples

### 1. Volume-based Configuration (Kubernetes)

```go
// Using volume-mounted ConfigMaps and Secrets
initOpts := opts.InitOption{
    Resources: []opts.ResourceOption{
        sailor.ConfigMapDefault(),  // Mounts from /etc/sailor/_config
        sailor.SecretsDefault(),    // Mounts from /etc/sailor/_secret
    },
}

consumer, err := sailor.NewConsumer[AppConfig, AppSecrets](initOpts)
```

### 2. Pull-based Configuration (Remote API)

```go
// Fetch configurations from remote Sailor API
initOpts := opts.InitOption{
    Connection: &opts.ConnectionOption{
        Addr:      "https://sailor.example.com",
        Namespace: "production",
        App:       "api-service",
        AccessKey: "your-access-key",
        SecretKey: "your-secret-key",
    },
    Resources: []opts.ResourceOption{
        sailor.ConfigPullDefault(),    // Pulls every 10 seconds
        sailor.SecretsPullDefault(),   // Pulls every 10 seconds
    },
}
```

### 3. Custom Resource Management

```go
// Manage custom resources with specific intervals
initOpts := opts.InitOption{
    Connection: &opts.ConnectionOption{
        Addr:      "https://sailor.example.com",
        Namespace: "my-app",
        App:       "web-service",
        AccessKey: "your-access-key",
        SecretKey: "your-secret-key",
    },
    Resources: []opts.ResourceOption{
        {
            Def: opts.ResourceDefinition{
                Kind: opts.CONFIGS,
            },
            FetchDef: opts.FetchDefinition{
                Fetch:        opts.PULL,
                PullInterval: 30 * time.Second,  // Custom interval
            },
            FallbackEnabled: true,
        },
        sailor.MiscPullDefault("certificates"),  // Custom misc resource
    },
}
```

### 4. Environment Variable Configuration

```go
// Use environment variables for connection
// Set these environment variables:
// SAILOR_URL=https://sailor.example.com
// SAILOR_NS=my-app
// SAILOR_APP=web-service
// SAILOR_ACCESS_KEY=your-access-key
// SAILOR_SECRET_KEY=your-secret-key

initOpts := opts.InitOption{
    Resources: []opts.ResourceOption{
        sailor.ConfigMapDefault(),
        sailor.SecretsDefault(),
    },
    // Connection will be read from environment variables
}

consumer, err := sailor.NewConsumer[AppConfig, AppSecrets](initOpts)
```

### 5. Mixed Resource Types

```go
// Combine different resource types
initOpts := opts.InitOption{
    Connection: &opts.ConnectionOption{
        Addr:      "https://sailor.example.com",
        Namespace: "production",
        App:       "api-service",
        AccessKey: "your-access-key",
        SecretKey: "your-secret-key",
    },
    Resources: []opts.ResourceOption{
        sailor.ConfigMapDefault(),           // Volume-mounted config
        sailor.SecretsPullDefault(),         // Pull-based secrets
        sailor.MiscOnceDefault("ssl-certs"), // One-time misc resource
    },
}
```

## 🔧 Configuration Options

### Resource Types

| Type      | Description                | Usage                            |
| --------- | -------------------------- | -------------------------------- |
| `CONFIGS` | Application configurations | `sailor.ConfigMapDefault()`      |
| `SECRETS` | Sensitive data             | `sailor.SecretsDefault()`        |
| `MISC`    | Custom resources           | `sailor.MiscOnceDefault("name")` |

### Fetch Methods

| Method   | Description               | Use Case                        |
| -------- | ------------------------- | ------------------------------- |
| `VOLUME` | Read from mounted volumes | Kubernetes ConfigMaps/Secrets   |
| `PULL`   | Fetch from remote API     | Remote configuration management |

### Default Functions

| Function                | Description       | Fetch Method | Interval   |
| ----------------------- | ----------------- | ------------ | ---------- |
| `ConfigMapDefault()`    | Default ConfigMap | Volume       | Real-time  |
| `SecretsDefault()`      | Default Secrets   | Volume       | Real-time  |
| `ConfigPullDefault()`   | Pull ConfigMap    | Pull         | 10 seconds |
| `SecretsPullDefault()`  | Pull Secrets      | Pull         | 10 seconds |
| `MiscOnceDefault(name)` | One-time Misc     | Pull         | Once       |
| `MiscPullDefault(name)` | Pull Misc         | Pull         | 10 seconds |

## 🛠️ Advanced Usage

### Custom Resource Paths

```go
customResource := opts.ResourceOption{
    Def: opts.ResourceDefinition{
        Kind: opts.CONFIGS,
        Path: "/custom/path",  // Custom mount path
    },
    FetchDef: opts.FetchDefinition{
        Fetch: opts.VOLUME,
    },
    FallbackEnabled: true,
}
```

### Custom Pull Intervals

```go
customPull := opts.ResourceOption{
    Def: opts.ResourceDefinition{
        Kind: opts.SECRETS,
    },
    FetchDef: opts.FetchDefinition{
        Fetch:        opts.PULL,
        PullInterval: 5 * time.Minute,  // Custom interval
    },
    FallbackEnabled: true,
}
```

### Accessing Misc Resources

```go
// Get raw bytes from misc resource
certBytes, err := consumer.GetMisc("ssl-certs")
if err != nil {
    log.Printf("Error getting certificates: %v", err)
    return
}

// Use the raw bytes as needed
// e.g., write to file, parse as PEM, etc.
```

## 🔒 Security

- **Type Safety**: All configurations are type-safe with compile-time checking
- **Atomic Operations**: Uses atomic pointers for thread-safe access
- **Secret Management**: Proper handling of sensitive data
- **Fallback Support**: Ensures high availability with fallback mechanisms

## 🐳 Kubernetes Integration

### ConfigMap Example

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
    name: my-app-config
data:
    _config: |
        {
          "database_url": "postgresql://localhost:5432/myapp",
          "api_key": "your-api-key",
          "port": 8080
        }
```

### Secret Example

```yaml
apiVersion: v1
kind: Secret
metadata:
    name: my-app-secrets
type: Opaque
data:
    _secret: |
        {
          "db_password": "base64-encoded-password",
          "jwt_secret": "base64-encoded-jwt-secret"
        }
```

### Deployment Example

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
    name: my-app
spec:
    template:
        spec:
            containers:
                - name: my-app
                  image: my-app:latest
                  volumeMounts:
                      - name: sailor-config
                        mountPath: /etc/sailor
            volumes:
                - name: sailor-config
                  configMap:
                      name: my-app-config
                - name: sailor-secrets
                  secret:
                      secretName: my-app-secrets
```

## 🚨 Error Handling

```go
consumer, err := sailor.NewConsumer[AppConfig, AppSecrets](initOpts)
if err != nil {
    switch {
    case errors.Is(err, sailor.ErrNewConsumerEmptyResourceList):
        log.Fatal("No resources specified")
    case errors.Is(err, sailor.ErrNewConsumerNoSailorURL):
        log.Fatal("Sailor URL not provided")
    case errors.Is(err, sailor.ErrNewConsumerNoSailorNS):
        log.Fatal("Namespace not provided")
    // ... handle other errors
    }
}

// Handle runtime errors
config, err := consumer.Get()
if err != nil {
    if errors.Is(err, sailor.ErrConfigsNotLoaded) {
        log.Printf("Configs not loaded yet")
        return
    }
    log.Printf("Error getting config: %v", err)
}
```

## 📖 API Reference

### Consumer Methods

| Method          | Description               | Returns           |
| --------------- | ------------------------- | ----------------- |
| `Get()`         | Get current configuration | `(C, error)`      |
| `GetSecret()`   | Get current secrets       | `(S, error)`      |
| `GetMisc(name)` | Get misc resource by name | `([]byte, error)` |

### Error Types

| Error                             | Description             |
| --------------------------------- | ----------------------- |
| `ErrNewConsumerEmptyResourceList` | No resources specified  |
| `ErrNewConsumerNoSailorURL`       | Sailor URL not provided |
| `ErrNewConsumerNoSailorNS`        | Namespace not provided  |
| `ErrNewConsumerNoSailorApp`       | App name not provided   |
| `ErrNewConsumerNoSailorAccessKey` | Access key not provided |
| `ErrNewConsumerNoSailorSecretKey` | Secret key not provided |
| `ErrConfigsNotLoaded`             | Configs not loaded      |
| `ErrSecretsNotLoaded`             | Secrets not loaded      |
| `ErrMiscNotLoaded`                | Misc resource not found |
| `ErrFetchFallbackFailed`          | Fallback fetch failed   |

## 🤝 Contributing

We welcome contributions! Please see our
[Contributing Guidelines](CONTRIBUTING.md) for details.

## 📄 License

This project is licensed under the GNU General Public License v3.0 - see the
[LICENSE](LICENSE) file for details.

## 👥 Authors

- **Ashish Shekar (codekidX)** - _Core development_

## 🙏 Acknowledgments

- Built with ❤️ for the Go and Kubernetes communities
- Inspired by the need for better configuration management in cloud-native
  applications

---

**Made with ⚡ by SailorHQ**
