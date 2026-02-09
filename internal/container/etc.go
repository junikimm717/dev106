package container

var (
	USERGROUPNAME = "dev106"
)

type EtcInfo struct {
	Passwd *PasswdInfo
	Group  *GroupInfo
	Shadow *ShadowInfo
}

func ReadEtc() (*EtcInfo, error) {
	passwd, err := ReadPasswd()
	if err != nil {
		return nil, err
	}
	group, err := ReadGroup()
	if err != nil {
		return nil, err
	}
	shadow, err := ReadShadow()
	if err != nil {
		return nil, err
	}
	return &EtcInfo{
		Passwd: passwd,
		Group:  group,
		Shadow: shadow,
	}, nil
}

func (e *EtcInfo) Clear(uid, gid int) {
	passwdentries := make([]*PasswdEntry, 0, len(e.Passwd.Entries))
	groupentries := make([]*GroupEntry, 0, len(e.Group.Entries))
	shadowentries := make([]*ShadowEntry, 0, len(e.Shadow.Entries))
	for _, entry := range e.Passwd.Entries {
		if entry.UID != uid && entry.Name != USERGROUPNAME {
			passwdentries = append(passwdentries, entry)
		}
	}
	for _, entry := range e.Group.Entries {
		if entry.GID != gid && entry.Name != USERGROUPNAME {
			groupentries = append(groupentries, entry)
		}
	}
	for _, entry := range e.Shadow.Entries {
		if entry.Name != USERGROUPNAME {
			shadowentries = append(shadowentries, entry)
		}
	}
	e.Passwd.Entries = passwdentries
	e.Group.Entries = groupentries
	e.Shadow.Entries = shadowentries
}

func (e *EtcInfo) Writeback() error {
	err := WritePasswd(e.Passwd)
	if err != nil {
		return err
	}
	err = WriteGroup(e.Group)
	if err != nil {
		return err
	}
	err = WriteShadow(e.Shadow)
	if err != nil {
		return err
	}
	return nil
}

func (e *EtcInfo) SetUIDGID(uid, gid int, homedir string) {
	// if dev106 group exists, its gid should be modified.
	e.Clear(uid, gid)
	e.Group.Entries = append(e.Group.Entries, &GroupEntry{
		Name:  USERGROUPNAME,
		GID:   gid,
		Users: []string{USERGROUPNAME},
	})
	e.Passwd.Entries = append(e.Passwd.Entries, &PasswdEntry{
		Name:    USERGROUPNAME,
		UID:     uid,
		GID:     gid,
		HomeDir: homedir,
		Shell:   e.Passwd.RootShell,
	})
	e.Shadow.Entries = append(e.Shadow.Entries, NewContainerShadowEntry(USERGROUPNAME))
}
