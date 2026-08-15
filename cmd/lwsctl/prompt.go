package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func prompt(message string) (string, error) {
	fmt.Fprint(os.Stderr, message)
	reader := bufio.NewReader(os.Stdin)
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(value), nil
}

func confirm(message string) (bool, error) {
	answer, err := prompt(message + " [y/N]: ")
	if err != nil {
		return false, err
	}
	return answer == "y" || answer == "Y" || answer == "yes" || answer == "YES", nil
}
