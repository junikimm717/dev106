package container

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

type GroupEntry struct {
	Name  string
	GID   int
	Users []string
}

type GroupInfo struct {
	Entries []*GroupEntry
}

func GroupEntrytoLine(entry *GroupEntry) string {
	return fmt.Sprintf(
		"%s:x:%d:%s",
		entry.Name,
		entry.GID,
		strings.Join(entry.Users, ","),
	)
}

func LinetoGroupEntry(line []byte) (*GroupEntry, error) {
	fields := bytes.Split(line, []byte(":"))
	if len(fields) != 4 {
		return nil, errors.New("Malformed Group Line:" + string(line))
	}
	gid, err := strconv.Atoi(string(fields[2]))
	if err != nil {
		return nil, err
	}
	return &GroupEntry{
		Name:  string(fields[0]),
		GID:   gid,
		Users: strings.Split(string(fields[3]), ","),
	}, nil
}

func ReadGroup() (*GroupInfo, error) {
	res := GroupInfo{}
	passwd, err := os.ReadFile("/etc/group")
	if err != nil {
		return nil, err
	}
	res.Entries = make([]*GroupEntry, 0, 20)
	for line := range bytes.SplitSeq(passwd, []byte("\n")) {
		entry, err := LinetoGroupEntry(line)
		if err != nil {
			log.Println(err)
			continue
		}
		res.Entries = append(res.Entries, entry)
	}
	return &res, nil
}

func WriteGroup(info *GroupInfo) error {
	f, err := os.OpenFile("/etc/group", os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, entry := range info.Entries {
		f.WriteString(GroupEntrytoLine(entry))
		f.WriteString("\n")
	}
	return nil
}
