package main

import (
	. "backend/internal/services/recognize"
)

func main() {
	Recognize(
		"ws://localhost:7880",
		"devkey",
		"secret",
		"myroom",
		"botuser",
	)
}
