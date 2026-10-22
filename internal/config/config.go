package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type DeployConfig struct {
	ScriptsDir string `yaml:"scripts_dir"`
}

type Config struct {
	Deploy   DeployConfig   `yaml:"deploy"`
	Server   ServerConfig   `yaml:"server"`
	Database DatabaseConfig `yaml:"database"`
	WorkDir  WorkDirConfig  `yaml:"work_dir"`
}

type WorkDirConfig struct {
	Path            string `yaml:"path"`
	ApplicationPath string `yaml:"application_path"`
	CIPath          string `yaml:"ci_path"`
}

type ServerConfig struct {
	Port      string `yaml:"port"`
	LoginPath string `yaml:"login_path"`
}

type DatabaseConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
	MaxConns int32  `yaml:"max_conns"`
}

func (d DatabaseConfig) DSN() string {
	sslmode := d.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}
	return fmt.Sprintf(
		"postgresql://%s:%s@%s:%d/%s?sslmode=%s",
		d.User, d.Password, d.Host, d.Port, d.DBName, sslmode,
	)
}

func Load() Config {
	cfg := defaults()

	if path := configPath(); path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config: read %s: %v, using defaults+env\n", path, err)
		} else if err := yaml.Unmarshal(data, &cfg); err != nil {
			fmt.Fprintf(os.Stderr, "config: parse %s: %v, using defaults+env\n", path, err)
		}
	}

	applyEnvOverrides(&cfg)
	return cfg
}

func defaults() Config {
	return Config{
		Deploy: DeployConfig{ScriptsDir: "deploy-scripts"},
		Server: ServerConfig{Port: "8080", LoginPath: "/bs"},
		Database: DatabaseConfig{
			Host:     "172.16.8.202",
			Port:     5432,
			User:     "postgres",
			Password: "CHANGE_ME",
			DBName:   "aether_ops",
			SSLMode:  "disable",
			MaxConns: 20,
		},
	}
}

func configPath() string {
	if p := os.Getenv("AETHER_CONFIG"); p != "" {
		return p
	}
	for _, name := range []string{"config.yaml", "config.yml"} {
		if _, err := os.Stat(name); err == nil {
			return name
		}
	}
	return ""
}

func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("AETHER_DEPLOY_SCRIPTS_DIR"); v != "" {
		cfg.Deploy.ScriptsDir = v
	}
	if v := os.Getenv("PORT"); v != "" {
		cfg.Server.Port = v
	}
	if v := os.Getenv("AETHER_LOGIN_PATH"); v != "" {
		cfg.Server.LoginPath = v
	}
	if v := os.Getenv("DB_HOST"); v != "" {
		cfg.Database.Host = v
	}
	if v := os.Getenv("DB_PORT"); v != "" {
		fmt.Sscanf(v, "%d", &cfg.Database.Port)
	}
	if v := os.Getenv("DB_USER"); v != "" {
		cfg.Database.User = v
	}
	if v := os.Getenv("DB_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}
	if v := os.Getenv("DB_NAME"); v != "" {
		cfg.Database.DBName = v
	}
	if v := os.Getenv("DB_SSLMODE"); v != "" {
		cfg.Database.SSLMode = v
	}
	if v := os.Getenv("WORK_DIR"); v != "" {
		cfg.WorkDir.Path = v
	}
	if v := os.Getenv("APP_WORK_DIR"); v != "" {
		cfg.WorkDir.ApplicationPath = v
	}
	if v := os.Getenv("CI_WORK_DIR"); v != "" {
		cfg.WorkDir.CIPath = v
	}
	cfg.WorkDir.normalize()
	cfg.Server.normalize()
}

func (w *WorkDirConfig) normalize() {
	if w.ApplicationPath == "" {
		w.ApplicationPath = w.Path
	}
	if w.CIPath == "" {
		w.CIPath = w.Path
	}
}

func (s *ServerConfig) normalize() {
	s.LoginPath = strings.TrimSpace(s.LoginPath)
	if s.LoginPath == "" {
		s.LoginPath = "/bs"
	}
	if !strings.HasPrefix(s.LoginPath, "/") {
		s.LoginPath = "/" + s.LoginPath
	}
}
