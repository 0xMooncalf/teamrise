package pipeline

import "fmt"

func Seed() []Candidate {
	names := []string{"Alex Morgan", "Jamie Park", "Sam Rivera", "Taylor Brooks", "Jordan Lee", "Casey Chen", "Robin Patel", "Avery Quinn", "Riley Santos", "Drew Harper", "Cameron Reed", "Skyler Kim", "Morgan Wells", "Blake Evans", "Finley Ross", "Hayden Cole", "Reese Bennett", "Parker Lane", "Sage Foster", "Emerson Gray", "Arden Bell", "Rowan Price", "Ellis Ward", "Remy Hayes", "Dakota Mills", "Quinn Stone", "River Woods", "Shiloh West", "Jules King", "Frankie James", "Charlie Ames", "Devon Cross", "Kendall Hart", "Lane Bishop", "Micah Snow", "Noel Ray", "Phoenix Shaw", "Tatum Ford", "Val Blair", "Winter Scott"}
	colors := []string{"#dce8e1", "#ebe1f4", "#e1eafb", "#f6e5d7", "#e7e5dd"}
	cs := make([]Candidate, 0, len(names))
	for i, name := range names {
		job, source := "Senior Go Engineer", "GitHub"
		if i%3 == 1 {
			job, source = "Community Manager", "Application"
		}
		if i%4 == 1 {
			job = "Frontend Engineer"
		}
		if i%4 == 2 {
			job, source = "Cleaning Specialist", "Application"
		}
		if i%4 == 0 {
			source = "Portfolio"
		}
		var a, b string
		fmt.Sscanf(name, "%s %s", &a, &b)
		cs = append(cs, Candidate{ID: fmt.Sprintf("candidate-%02d", i+1), Name: name, Initials: a[:1] + b[:1], Job: job, Source: source, Stage: "Sourced", Role: "—", Experience: 3 + i%9, Color: colors[i%len(colors)], NoShow: i%9 == 8, Transcript: []Message{}, Scorecard: []Score{}, Feedback: []string{}, ProfileID: profileIDForJob(job)})
	}
	return cs
}

func DemoTranscript(c Candidate) []Message {
	return PlannedTranscript(c)
}
