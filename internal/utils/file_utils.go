package utils

import (
	"bufio"
	"fmt"
	"os"
)

func ReadFile(filePath string) ([]string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	result := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		result = append(result, line)
	}

	if scanner.Err() != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return result, nil
}
