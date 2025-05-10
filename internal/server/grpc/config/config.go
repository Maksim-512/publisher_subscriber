package config

import (
	"fmt"

	"github.com/ilyakaznacheev/cleanenv"
)

type Config struct {
	GRPC GRPCConfig `yaml:"grpc"`
}

type GRPCConfig struct {
	Port string `yaml:"port"`
}

func Load(path string) (*Config, error) {
	var cfg Config

	err := cleanenv.ReadConfig(path, &cfg)
	if err != nil {
		fmt.Println(path, err)
		return nil, err
	}

	return &cfg, nil
}
