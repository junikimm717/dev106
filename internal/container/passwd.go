package container

import (
	"bytes"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
)

type PasswdEntry struct {
	Name    string
	UID     int
	GID     int
	HomeDir string
	Shell   string
}

type PasswdInfo struct {
	RootShell string
	Entries   []*PasswdEntry
}

func LinetoPasswdEntry(line []byte) (*PasswdEntry, error) {
	fields := bytes.Split(line, []byte(":"))
	if len(fields) != 7 {
		return nil, errors.New("Malformed Group Line:" + string(line))
	}
	uid, err := strconv.Atoi(string(fields[2]))
	if err != nil {
		return nil, err
	}
	gid, err := strconv.Atoi(string(fields[3]))
	if err != nil {
		return nil, err
	}
	return &PasswdEntry{
		Name:    string(fields[0]),
		UID:     uid,
		GID:     gid,
		HomeDir: string(fields[5]),
		Shell:   string(fields[6]),
	}, nil
}

func PasswdEntrytoLine(entry *PasswdEntry) string {
	return fmt.Sprintf(
		"%s:x:%d:%d:user:%s:%s",
		entry.Name,
		entry.UID,
		entry.GID,
		entry.HomeDir,
		entry.Shell,
	)
}

func ReadPasswd() (*PasswdInfo, error) {
	res := PasswdInfo{}
	passwd, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil, err
	}
	res.Entries = make([]*PasswdEntry, 0, 20)
	UIDMap := make(map[int]*PasswdEntry)
	for line := range bytes.SplitSeq(passwd, []byte("\n")) {
		entry, err := LinetoPasswdEntry(line)
		if err != nil {
			log.Println(err)
			continue
		}
		res.Entries = append(res.Entries, entry)
		UIDMap[entry.UID] = entry
	}
	if entry, ok := UIDMap[0]; ok {
		res.RootShell = entry.Shell
	} else {
		return nil, errors.New("UID 0 not found in /etc/passwd!")
	}
	return &res, nil
}

func WritePasswd(info *PasswdInfo) error {
	f, err := os.OpenFile("/etc/passwd", os.O_RDWR|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, entry := range info.Entries {
		f.WriteString(PasswdEntrytoLine(entry))
		f.WriteString("\n")
	}
	return nil
}
