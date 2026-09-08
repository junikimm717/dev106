package cli

import (
	"fmt"

	"github.com/junikimm717/dev106/internal/tui"
)

type courseOption struct {
	Name       string
	Label      string
	Image      string
	FollowHost bool
}

var courseOptions = []courseOption{
	{
		Name:       "6.181",
		Label:      "6.181  Operating System Engineering (xv6 / RISC-V)",
		Image:      "ghcr.io/junikimm717/dev106/nvim_6181:latest",
		FollowHost: true,
	},
	{
		Name:       "6.106",
		Label:      "6.106  Software Performance Engineering",
		Image:      "ghcr.io/junikimm717/dev106/nvim:2.1.0",
		FollowHost: false,
	},
}

func defaultCourse() courseOption {
	return courseOptions[0]
}

func resolveCourse() (courseOption, error) {
	if !tui.HasTTY() {
		return defaultCourse(), nil
	}

	choices := make([]tui.Choice, len(courseOptions))
	for i, course := range courseOptions {
		choices[i] = tui.Choice{ID: course.Name, Label: course.Label}
	}

	selected, err := tui.Select("Which course are you taking?", choices)
	if err != nil {
		return courseOption{}, err
	}

	for _, course := range courseOptions {
		if course.Name == selected.ID {
			return course, nil
		}
	}
	return courseOption{}, fmt.Errorf("unknown course %q", selected.ID)
}
