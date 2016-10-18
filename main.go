package main

import (
	"os"

	"github.com/carlpett/prombeat/beater"

	"github.com/elastic/beats/libbeat/beat"
)

func main() {
	err := beat.Run("prombeat", "", beater.New)
	if err != nil {
		os.Exit(1)
	}
}
