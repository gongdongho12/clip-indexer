package media

import "strings"

type groupRule struct {
	keywords []string
	info     GroupInfo
}

var groupRules = []groupRule{
	{keywords: []string{"airport", "terminal", "flight", "airplane", "kansai_airport", "kix", "공항", "비행기", "터미널"}, info: GroupInfo{Key: "airport", Label: "Airport", Folder: "airport"}},
	{keywords: []string{"train", "station", "subway", "metro", "rail", "haruka", "ticket_machine", "transportation", "ticket vending", "기차", "열차", "역", "지하철", "전철"}, info: GroupInfo{Key: "train", Label: "Train", Folder: "train"}},
	{keywords: []string{"bus", "taxi", "car", "driving", "ferry", "boat", "transport", "버스", "택시", "자동차", "운전", "배"}, info: GroupInfo{Key: "transport", Label: "Transport", Folder: "transport"}},
	{keywords: []string{"restaurant", "cafe", "food", "meal", "breakfast", "lunch", "dinner", "ramen", "sushi", "coffee", "bar", "식당", "카페", "음식", "밥", "라멘", "초밥", "커피"}, info: GroupInfo{Key: "food", Label: "Food", Folder: "food"}},
	{keywords: []string{"hotel", "room", "lobby", "accommodation", "checkin", "check-in", "호텔", "숙소", "객실", "로비"}, info: GroupInfo{Key: "hotel", Label: "Hotel", Folder: "hotel"}},
	{keywords: []string{"temple", "shrine", "castle", "bridge", "landmark", "observatory", "museum", "tower", "akashi_kaikyo_bridge", "사찰", "절", "신사", "성", "다리", "박물관", "전망대"}, info: GroupInfo{Key: "landmark", Label: "Landmark", Folder: "landmark"}},
	{keywords: []string{"beach", "sea", "ocean", "mountain", "park", "forest", "coastal", "lake", "river", "sunset", "scenic", "view", "바다", "해변", "산", "공원", "숲", "강", "노을", "풍경"}, info: GroupInfo{Key: "nature", Label: "Nature", Folder: "nature"}},
	{keywords: []string{"shop", "shopping", "market", "store", "mall", "convenience", "쇼핑", "시장", "상점", "편의점", "마트"}, info: GroupInfo{Key: "shopping", Label: "Shopping", Folder: "shopping"}},
	{keywords: []string{"street", "city", "downtown", "night_view", "walking", "neighborhood", "urban", "거리", "도시", "야경", "산책", "시내"}, info: GroupInfo{Key: "city", Label: "City", Folder: "city"}},
	{keywords: []string{"people", "family", "friend", "portrait", "selfie", "사람", "가족", "친구", "인물", "셀피"}, info: GroupInfo{Key: "people", Label: "People", Folder: "people"}},
}

func updateItemGroup(item *Item) {
	group := groupForItem(*item)
	item.Group = &group
}

func updateItemGroups(items []Item) {
	for index := range items {
		updateItemGroup(&items[index])
	}
}

func groupForItem(item Item) GroupInfo {
	text := strings.ToLower(strings.Join(groupTextParts(item), " "))
	for _, rule := range groupRules {
		for _, keyword := range rule.keywords {
			if strings.Contains(text, strings.ToLower(keyword)) {
				info := rule.info
				info.Reason = keyword
				return info
			}
		}
	}
	return GroupInfo{Key: "other", Label: "Other", Folder: "other", Reason: "fallback"}
}

func groupTextParts(item Item) []string {
	parts := append([]string{}, item.Tags...)
	if item.Location != nil {
		parts = append(parts, item.Location.Label, item.Location.Notes)
	}
	if item.Content != nil {
		parts = append(parts,
			item.Content.SceneSummary,
			item.Content.AudioSummary,
			item.Content.AudioTranscript,
			item.Content.LocationGuess,
			item.Content.Notes,
		)
		parts = append(parts, item.Content.Tags...)
		parts = append(parts, item.Content.AudioTags...)
	}
	return parts
}
