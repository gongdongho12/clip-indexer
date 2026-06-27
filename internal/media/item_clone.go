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

func cloneReport(report Report) Report {
	next := report
	next.Items = make([]Item, 0, len(report.Items))
	for _, item := range report.Items {
		next.Items = append(next.Items, cloneItem(item))
	}
	next.Warnings = append([]string{}, report.Warnings...)
	next.FolderTree = cloneFolderTree(report.FolderTree)
	return next
}

func cloneFolderTree(nodes []FolderTreeNode) []FolderTreeNode {
	if len(nodes) == 0 {
		return nil
	}
	next := make([]FolderTreeNode, 0, len(nodes))
	for _, node := range nodes {
		cloned := node
		cloned.Files = append([]string{}, node.Files...)
		cloned.Children = cloneFolderTree(node.Children)
		next = append(next, cloned)
	}
	return next
}
