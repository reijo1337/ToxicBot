package utils

import (
	"bufio"
	"fmt"
	"os"
)

func ReadFile(filePath string, list *[]string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		*list = append(*list, line)
	}

	if scanner.Err() != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	return nil
}
