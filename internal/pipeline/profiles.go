package pipeline

import (
	"fmt"
	"strings"
)

func DefaultProfiles() []InterviewProfile {
	return []InterviewProfile{
		{ID: "backend", Name: "Backend engineer", Track: "backend", SkillPrompt: "You are a backend interviewing specialist. Assess API design, Go concurrency, reliability, and practical debugging. Ask one question at a time, follow up on evidence, and produce a short grounded assessment. Escalate uncertainty to a human.", Questions: []string{"How would you prevent duplicate updates when multiple workers process the same request?", "A service gets slower under load. What would you inspect first, and why?", "How do you handle an API request that timed out after the remote action might already have completed?"}, Criteria: []string{"Reliability", "Problem solving", "Communication"}},
		{ID: "frontend", Name: "Frontend engineer", Track: "frontend", SkillPrompt: "You are a frontend interviewing specialist. Assess React, accessibility, browser performance, and maintainable UI architecture. Ask for concrete examples and tradeoffs. Avoid assumptions about the candidate and leave the final decision to a human.", Questions: []string{"How would you make a modal usable with a keyboard and a screen reader?", "A React page re-renders too often. How would you find and fix the cause?", "How would you handle loading, empty, and failure states in a candidate dashboard?"}, Criteria: []string{"UI engineering", "Accessibility", "Communication"}},
		{ID: "cleaning", Name: "Cleaning specialist", Track: "cleaning", SkillPrompt: "You are interviewing a cleaning specialist. Focus on safe working practices, following instructions, equipment care, and handling unexpected problems. Use clear everyday language, avoid technical software questions, and ask for practical examples.", Questions: []string{"You find an unlabeled cleaning product. What do you do before using it?", "How would you clean a busy area while keeping people safe?", "A customer reports that a room was missed. How would you handle it?"}, Criteria: []string{"Safe working practices", "Work organization", "Communication"}},
		{ID: "community", Name: "Community manager", Track: "general", SkillPrompt: "You are interviewing a community manager. Explore moderation, conflict resolution, planning, and member wellbeing using concrete situations. Escalate ambiguous cases and preserve evidence.", Questions: []string{"A member reports harassment during a live event. What are your first steps?", "How would you measure whether a community is healthy?", "Two moderators disagree on a decision. How would you help resolve it?"}, Criteria: []string{"Judgment", "Organization", "Communication"}},
	}
}

func validateProfile(p InterviewProfile) error {
	if len(p.ID) < 1 || len(p.ID) > 64 || len(strings.TrimSpace(p.Name)) < 2 || len(p.Name) > 100 || len(strings.TrimSpace(p.SkillPrompt)) < 20 || len(p.SkillPrompt) > 8000 {
		return fmt.Errorf("profile needs an ID, a name, and a skill prompt of 20–8000 characters")
	}
	for _, r := range p.ID {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return fmt.Errorf("profile ID must use lowercase letters, digits, and hyphens")
		}
	}
	if p.Track != "backend" && p.Track != "frontend" && p.Track != "cleaning" && p.Track != "general" {
		return fmt.Errorf("choose a valid demo skill track")
	}
	if len(p.Questions) < 1 || len(p.Questions) > 8 || len(p.Criteria) < 1 || len(p.Criteria) > 6 {
		return fmt.Errorf("use 1–8 questions and 1–6 assessment criteria")
	}
	for _, s := range append(append([]string{}, p.Questions...), p.Criteria...) {
		if len([]rune(strings.TrimSpace(s))) < 3 || len([]rune(s)) > 600 {
			return fmt.Errorf("questions and criteria must contain 3–600 characters")
		}
	}
	seen := make(map[string]bool, len(p.Cards))
	for _, card := range p.Cards {
		if len(card.ID) < 1 || len(card.ID) > 64 || seen[card.ID] ||
			(card.Channel != "voice" && card.Channel != "chat") ||
			(card.Image != "" && card.Channel != "chat") || len([]rune(card.Notes)) > 1000 {
			return fmt.Errorf("each question card needs a distinct ID, a valid format and up to 1000 characters of notes; screenshots require Discord chat")
		}
		seen[card.ID] = true
	}
	return nil
}

func profileIDForJob(job string) string {
	j := strings.ToLower(job)
	if strings.Contains(j, "frontend") {
		return "frontend"
	}
	if strings.Contains(j, "clean") {
		return "cleaning"
	}
	if strings.Contains(j, "community") {
		return "community"
	}
	return "backend"
}

func PlannedTranscript(c Candidate) []Message {
	p := c.Profile
	result := []Message{}
	answers := map[string][]string{
		"backend":  {"I would use an idempotency key and serialize the state transition. I would record the operation durably before publishing the result, then reconcile retries against that record.", "I would start with request latency, profiles, and database timings. I would reproduce the bottleneck under representative load before changing the code.", "I would treat the outcome as unknown, check the remote state, and retry only when the action is safe to repeat. I would explain the uncertainty instead of hiding it."},
		"frontend": {"I would move focus into the dialog, keep keyboard navigation inside it, label the dialog clearly, and restore focus when it closes. I would also test it with a screen reader.", "I would use the React profiler to find which changes cause work. Then I would reduce unnecessary state updates and measure the result before adding memoization.", "I would show a clear loading state, an actionable empty state, and an error with a retry option. Updates should preserve focus and announce meaningful changes."},
		"cleaning": {"I would not use an unlabeled product. I would set it aside safely and ask the supervisor to identify it and provide the correct instructions and protective equipment.", "I would mark the area, keep a safe route open, and work in small sections. I would follow the product instructions and never mix cleaning chemicals.", "I would acknowledge the issue, check what was missed, and arrange to clean it. I would tell the supervisor and review the checklist so it is less likely to happen again."},
		"general":  {"I would acknowledge the report, preserve the relevant evidence, and follow the published policy. If the situation is ambiguous, I would ask a human lead to review it.", "I would combine participation and response-time data with direct feedback. One number alone would not show whether people are receiving useful support.", "I would ask each person for the evidence and the rule they applied, then work toward a documented decision. I would escalate if we could not resolve it fairly."},
	}
	for i, q := range p.Questions {
		list, ok := answers[p.Track]
		if !ok {
			list = answers["general"]
		}
		text := list[i%len(list)]
		defaults := false
		for _, preset := range DefaultProfiles() {
			if preset.ID == p.ID && i < len(preset.Questions) && preset.Questions[i] == q {
				defaults = true
			}
		}
		if !defaults {
			text = fmt.Sprintf("For this %s scenario, I would clarify the requirements, explain a concrete approach, check the risks, and ask for help when the evidence is incomplete. This is a synthetic answer to: %s", p.Name, q)
		}
		result = append(result, Message{Speaker: "Interviewer", Text: q}, Message{Speaker: c.Name, Text: text, ResponseSeconds: 5 + (candidateNumber(c)+i*3)%12})
	}
	return result
}

func assessmentFor(c Candidate) (*Assessment, []Score, int) {
	n := candidateNumber(c)
	score := 68 + (n*7)%29
	total, count := 0, 0
	for _, m := range c.Transcript {
		if m.Speaker == c.Name {
			total += m.ResponseSeconds
			count++
		}
	}
	avg := 0
	if count > 0 {
		avg = total / count
	}
	rubric := []Score{}
	for i, label := range c.Profile.Criteria {
		value := max(0, min(100, score+(i%3-1)*2))
		evidence := "Insufficient response evidence."
		if len(c.Transcript) > 1 {
			evidence = c.Transcript[min(len(c.Transcript)-1, 1+(i%max(1, len(c.Transcript)/2))*2)].Text
		}
		rubric = append(rubric, Score{Label: label, Value: value, Evidence: evidence})
	}
	a := &Assessment{Summary: fmt.Sprintf("Provides a structured approach to %s questions and describes when to seek help.", strings.ToLower(c.Profile.Name)), Characterization: fmt.Sprintf("Responds to all %d scripted questions, explains practical steps, and acknowledges uncertainty. Average modeled response delay: %d seconds. These are scenario observations, not a personality assessment.", count, avg), Recommendation: "Invite to a final human interview and validate the examples against the role requirements.", Strengths: []string{"Describes an ordered approach", "Recognizes when to escalate", "Addresses the follow-up questions"}, Followups: []string{"Ask for one example from previous work", "Validate the proposed approach with a role-specific exercise"}, AverageResponseSeconds: avg}
	if score < 80 || n%7 == 0 {
		a.Summary = "Gives plausible answers, but the scenario does not contain enough independently supported examples."
		a.Recommendation = "Request a human review and ask for one more concrete example before making a decision."
	}
	return a, rubric, score
}
