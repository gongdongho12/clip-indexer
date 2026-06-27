package media

func cloneItem(item Item) Item {
	next := item
	next.Tags = append([]string{}, item.Tags...)
	next.Warnings = append([]string{}, item.Warnings...)
	if item.Video != nil {
		video := *item.Video
		next.Video = &video
	}
	if item.Audio != nil {
		audio := *item.Audio
		next.Audio = &audio
	}
	if item.Location != nil {
		location := *item.Location
		next.Location = &location
	}
	if item.Content != nil {
		content := *item.Content
		content.Tags = append([]string{}, item.Content.Tags...)
		content.AudioTags = append([]string{}, item.Content.AudioTags...)
		next.Content = &content
	}
	if item.Group != nil {
		group := *item.Group
		next.Group = &group
	}
	return next
}
