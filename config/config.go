package config

import (
	"time"
)

type Config struct {
	Address  string        `config:"address"`
	Period   time.Duration `config:"period"`
	Queries  []Query       `config:"queries"`
	Matchers []string      `config:"matchers"`
}
type Query struct {
	Name  string `config:"name"`
	Query string `config:"query"`
}

var DefaultConfig = Config{
	Address: "http://localhost:9090",
	Period:  1 * time.Second,
}
