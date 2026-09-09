package cli

import (
	"errors"
	"fmt"

	"github.com/junikimm717/dev106/internal/tui"
)

type courseOption struct {
	Name       string
	Label      string
	Image      string
	FollowHost bool
	Telerun    bool
}

var courseOptions = []courseOption{
	{
		Name:       "6.181",
		Label:      "6.1810 Operating System Engineering",
		Image:      "ghcr.io/junikimm717/dev106/mit_6181:latest",
		FollowHost: true,
		Telerun:    false,
	},
	{
		Name:       "6.106",
		Label:      "6.1060 Software Performance Engineering",
		Image:      "ghcr.io/junikimm717/dev106/mit_6106:2.1.0",
		FollowHost: false,
		Telerun:    true,
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
		if errors.Is(err, tui.ErrCancelled) {
			return courseOption{}, errors.New("setup cancelled; no config written")
		}
		return courseOption{}, err
	}

	for _, course := range courseOptions {
		if course.Name == selected.ID {
			return course, nil
		}
	}
	return courseOption{}, fmt.Errorf("unknown course %q", selected.ID)
}
