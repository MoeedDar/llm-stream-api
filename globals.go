package main

import (
	"os"
	"time"
)

const (
	maxConcurrent = 1
	llmTimeout    = 10 * time.Second
)

var (
	llmHost     = os.Getenv("LLM_HOST")
	secret      = os.Getenv("JWT_SECRET")
	allowOrigin = os.Getenv("ALLOW_ORIGIN")
)
