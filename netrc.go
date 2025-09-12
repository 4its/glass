package main

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func defaultNetrcPath() string {
	if h, err := os.UserHomeDir(); err == nil {
		return filepath.Join(h, ".netrc")
	}
	return ".netrc"
}

type netrcEntry struct {
	Machine  string
	Login    string
	Password string
}

func parseNetrc(r io.Reader) ([]netrcEntry, error) {
	var entries []netrcEntry
	var cur *netrcEntry

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		// обрезаем комментарий
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = line[:idx]
		}
		fields := strings.Fields(line)
		i := 0
		for i < len(fields) {
			switch fields[i] {
			case "machine":
				if i+1 < len(fields) {
					if cur != nil {
						entries = append(entries, *cur)
					}
					cur = &netrcEntry{Machine: fields[i+1]}
					i += 2
					continue
				}
			case "login":
				if cur != nil && i+1 < len(fields) {
					cur.Login = fields[i+1]
					i += 2
					continue
				}
			case "password":
				if cur != nil && i+1 < len(fields) {
					cur.Password = fields[i+1]
					i += 2
					continue
				}
			default:
				i++
			}
		}
	}
	if cur != nil {
		entries = append(entries, *cur)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

func loadTokenFromNetrc(netrcPath, host string) (string, bool, error) {
	f, err := os.Open(netrcPath)
	if err != nil {
		return "", false, err
	}
	defer f.Close()
	entries, err := parseNetrc(f)
	if err != nil {
		return "", false, err
	}
	for _, e := range entries {
		if e.Machine == host {
			return e.Password, true, nil
		}
	}
	return "", false, nil
}
