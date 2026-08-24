package tui

import (
	"time"
)

func (m *tuiModel) loadMoreGoalWeeks(count int) {
	if len(m.goalWeeks) == 0 {
		return
	}
	last := m.goalWeeks[len(m.goalWeeks)-1]
	firstStart, err := firstWeekStart()
	if err != nil {
		m.err = err
		return
	}
	if firstStart != "" && last.weekStart <= firstStart {
		return
	}
	t, _ := time.Parse("2006-01-02", last.weekStart)
	for i := 1; i <= count; i++ {
		ws := t.AddDate(0, 0, -7*i).Format("2006-01-02")
		w, err := loadGoalWeek(ws, weekNumFor(ws, firstStart))
		if err != nil {
			m.err = err
			return
		}
		m.goalWeeks = append(m.goalWeeks, w)
		if firstStart != "" && ws <= firstStart {
			break
		}
	}
}

func (m tuiModel) currentGoalWeek() *goalsWeek {
	if m.goalCurWeek < 0 || m.goalCurWeek >= len(m.goalWeeks) {
		return nil
	}
	return &m.goalWeeks[m.goalCurWeek]
}

func (m *tuiModel) goalUp() {
	if len(m.goalWeeks) == 0 {
		return
	}
	if m.goalCurGoal > 0 {
		m.goalCurGoal--
		return
	}
	if m.goalCurGoal == 0 {
		m.goalCurGoal = -1
		return
	}
	if m.goalCurDay >= 0 {
		if m.goalCurDay > 0 {
			m.goalCurDay--
			if m.goalCurWeek == 0 && m.goalCurDay < m.goalRevealFrom {
				m.goalRevealFrom = m.goalCurDay
			}
			m.goalCurGoal = -1
			return
		}
		m.goalCurDay = -1
		return
	}
	if m.goalCurWeek > 0 {
		prev := m.goalCurWeek - 1
		m.goalCurWeek = prev
		if prev == m.goalWeekExpanded {
			w := m.goalWeeks[prev]
			m.goalCurDay = 6
			if len(w.days[6].goals) > 0 {
				m.goalCurGoal = len(w.days[6].goals) - 1
			} else {
				m.goalCurGoal = -1
			}
		}
	}
}

func (m *tuiModel) goalDown() {
	if len(m.goalWeeks) == 0 {
		return
	}
	if m.goalCurDay >= 0 {
		w := m.goalWeeks[m.goalCurWeek]
		day := w.days[m.goalCurDay]
		if m.goalCurGoal < len(day.goals)-1 {
			m.goalCurGoal++
			return
		}
		if m.goalCurDay < 6 {
			m.goalCurDay++
			m.goalCurGoal = -1
			return
		}
		m.goalAdvanceWeek()
		return
	}
	if m.goalCurWeek == m.goalWeekExpanded {
		start := 0
		if m.goalCurWeek == 0 {
			start = m.goalRevealFrom
		}
		m.goalCurDay = start
		m.goalCurGoal = -1
		return
	}
	m.goalAdvanceWeek()
}

func (m *tuiModel) goalAdvanceWeek() {
	if m.goalCurWeek >= len(m.goalWeeks)-1 {
		m.loadMoreGoalWeeks(4)
	}
	if m.goalCurWeek < len(m.goalWeeks)-1 {
		m.goalCurWeek++
		m.goalCurDay = -1
		m.goalCurGoal = -1
	}
}
