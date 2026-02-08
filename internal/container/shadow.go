package container

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
)

type ShadowEntry struct {
	Name          string
	PasswordHash  string
	LastChange    int
	MinAge        int
	MaxAge        int
	WarnPeriod    int
	Inactivity    string
	Expire        string
	ReservedField string
}

type ShadowInfo struct {
	Entries []*ShadowEntry
}

func ShadowEntryToLine(entry *ShadowEntry) string {
	return fmt.Sprintf(
		"%s:%s:%d:%d:%d:%d:%s:%s:%s",
		entry.Name,
		entry.PasswordHash,
		entry.LastChange,
		entry.MinAge,
		entry.MaxAge,
		entry.WarnPeriod,
		entry.Inactivity,
		entry.Expire,
		entry.ReservedField,
	)
}

func LineToShadowEntry(line []byte) (*ShadowEntry, error) {
	fields := bytes.Split(line, []byte(":"))
	if len(fields) != 9 {
		return nil, errors.New("Malformed Shadow Line: " + string(line))
	}

	lastChange, err := strconv.Atoi(string(fields[2]))
	if err != nil && len(fields[2]) != 0 {
		return nil, err
	}

	minAge, err := strconv.Atoi(string(fields[3]))
	if err != nil && len(fields[3]) != 0 {
		return nil, err
	}

	maxAge, err := strconv.Atoi(string(fields[4]))
	if err != nil && len(fields[4]) != 0 {
		return nil, err
	}

	warn, err := strconv.Atoi(string(fields[5]))
	if err != nil && len(fields[5]) != 0 {
		return nil, err
	}

	return &ShadowEntry{
		Name:          string(fields[0]),
		PasswordHash:  string(fields[1]),
		LastChange:    lastChange,
		MinAge:        minAge,
		MaxAge:        maxAge,
		WarnPeriod:    warn,
		Inactivity:    string(fields[6]),
		Expire:        string(fields[7]),
		ReservedField: string(fields[8]),
	}, nil
}

func ReadShadow() (*ShadowInfo, error) {
	res := ShadowInfo{}

	data, err := os.ReadFile("/etc/shadow")
	if err != nil {
		return nil, err
	}

	res.Entries = make([]*ShadowEntry, 0, 20)

	for line := range bytes.SplitSeq(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}

		entry, err := LineToShadowEntry(line)
		if err != nil {
			log.Println(err)
			continue
		}

		res.Entries = append(res.Entries, entry)
	}

	return &res, nil
}

func WriteShadow(info *ShadowInfo) error {
	f, err := os.OpenFile("/etc/shadow", os.O_RDWR|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()

	for _, entry := range info.Entries {
		f.WriteString(ShadowEntryToLine(entry))
		f.WriteString("\n")
	}

	return nil
}

func NewContainerShadowEntry(username string) *ShadowEntry {
	return &ShadowEntry{
		Name:          username,
		PasswordHash:  "*", // IMPORTANT: not "!"
		LastChange:    19300,
		MinAge:        0,
		MaxAge:        99999,
		WarnPeriod:    7,
		Inactivity:    "",
		Expire:        "",
		ReservedField: "",
	}
}
