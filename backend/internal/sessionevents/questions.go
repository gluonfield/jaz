package sessionevents

func QuestionOpen(question ACPPermission) bool {
	return len(question.Questions) > 0 && question.Status != "selected" && question.Status != "cancelled"
}

func Questions(events []Event) []ACPPermission {
	questions := []ACPPermission{}
	byID := map[string]int{}
	for _, event := range events {
		permission := event.Permission
		if permission == nil {
			continue
		}
		if event.Type == "permission_request" && len(permission.Questions) > 0 {
			byID[permission.ID] = len(questions)
			questions = append(questions, *permission)
		}
		if index, exists := byID[permission.ID]; exists && event.Type == "permission_response" {
			question := &questions[index]
			if permission.Status == "selected" || permission.Status == "cancelled" {
				question.Status = permission.Status
				question.Origin = permission.Origin
				question.Answers = permission.Answers
				question.SelectedOptionID = permission.SelectedOptionID
			}
		}
	}
	return questions
}
