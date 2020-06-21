package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSign(t *testing.T) {
	url := "/api/v3/suggesters/search"
	method := "get"
	timestamp := "1565827270558"
	sig := sign(url, method, timestamp)
	assert.Equal(t, "6iU/x0HyEqX2dzMTdv1QsTtBX4Z8tZTuHJmhzMXnxuU=", sig)
}
