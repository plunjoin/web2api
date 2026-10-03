package geminiweb

// BuiltinModel 内置模型定义（模型发现失败时的兜底）。
type BuiltinModel struct {
	ModelID     string
	Name        string
	DisplayName string
	Description string
	Capacity    int
	ModelNumber int
}

// BuiltinModels 默认模型表（对应 gemini_webapi.constants.Model）。
var BuiltinModels = []BuiltinModel{
	{ModelID: "fbb127bbb056c959", Name: "gemini-flash", DisplayName: "Gemini Flash", Description: "Fast, lightweight model", Capacity: 1, ModelNumber: 1},
	{ModelID: "cf41b0e0dd7d53e5", Name: "gemini-flash-lite", DisplayName: "Gemini Flash Lite", Description: "Fastest, lowest latency", Capacity: 1, ModelNumber: 6},
	{ModelID: "9d8ca3786ebdfbea", Name: "gemini-pro", DisplayName: "Gemini Pro", Description: "Advanced reasoning model", Capacity: 1, ModelNumber: 3},
}

// AvailableModel 动态发现的模型。
type AvailableModel struct {
	ID            string
	Name          string
	DisplayName   string
	Description   string
	Capacity      int
	CapacityField int
	ModelNumber   int
	Available     bool
	Aliases       []string
}

// Header 构建模型选择请求头（x-goog-ext-525001261-jspb）。
func (m *AvailableModel) Header() map[string]string {
	return buildModelHeaders(m.ID, m.Capacity, m.ModelNumber)
}

// computeCapacity 由账号 tier/capability 标志推导容量（参考 AvailableModel.compute_capacity）。
func computeCapacity(tierFlags, capabilityFlags []any) (int, int) {
	has := func(list []any, target int) bool {
		for _, v := range list {
			if n, ok := toInt(v); ok && n == target {
				return true
			}
		}
		return false
	}
	if has(tierFlags, 21) {
		return 1, 13
	}
	if has(tierFlags, 22) {
		return 2, 13
	}
	if has(capabilityFlags, 115) {
		return 4, 12
	}
	if has(tierFlags, 16) || has(capabilityFlags, 106) {
		return 3, 12
	}
	if has(tierFlags, 8) || has(capabilityFlags, 19) {
		return 2, 12
	}
	return 1, 12
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	}
	return 0, false
}

// slugify 转小写并替换空格为连字符。
func slugify(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			out = append(out, r+('a'-'A'))
		} else if r == ' ' {
			out = append(out, '-')
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}

// parseModelsFromRPC 从 GetUserStatus RPC 响应提取模型列表。
func parseModelsFromRPC(frames []any) []*AvailableModel {
	var out []*AvailableModel
	for _, frame := range frames {
		part, ok := frame.([]any)
		if !ok || len(part) < 3 {
			continue
		}
		bodyStr, _ := part[2].(string)
		if bodyStr == "" {
			continue
		}
		var body any
		if err := jsonUnmarshal([]byte(bodyStr), &body); err != nil {
			continue
		}
		list, ok := body.([]any)
		if !ok {
			continue
		}
		modelsData := getNested(list, nil, 15)
		modelsList, ok := modelsData.([]any)
		if !ok {
			continue
		}
		tier, _ := getNested(list, []any{}, 16).([]any)
		capFlags, _ := getNested(list, []any{}, 17).([]any)
		capacity, field := computeCapacity(tier, capFlags)

		for _, md := range modelsList {
			mdata, ok := md.([]any)
			if !ok || len(mdata) == 0 {
				continue
			}
			modelID := strAt(mdata, 0)
			if modelID == "" {
				continue
			}
			category := strAt(mdata, 1)
			if category == "" {
				category = strAt(mdata, 10)
			}
			displayName := strAt(mdata, 11)
			if displayName == "" {
				displayName = strAt(mdata, 19)
			}
			if displayName == "" {
				displayName = category
			}
			description := strAt(mdata, 12)
			if description == "" {
				description = strAt(mdata, 2)
			}
			modelNumber := 1
			if n, ok := toInt(getNested(mdata, nil, 17)); ok && n > 0 {
				modelNumber = n
			} else if n, ok := toInt(getNested(mdata, nil, 9)); ok && n > 0 {
				modelNumber = n
			}
			name := "gemini-" + slugify(category)
			out = append(out, &AvailableModel{
				ID:            modelID,
				Name:          name,
				DisplayName:   displayName,
				Description:   description,
				Capacity:      capacity,
				CapacityField: field,
				ModelNumber:   modelNumber,
				Available:     true,
				Aliases:       []string{name, slugify(displayName), modelID},
			})
		}
		if len(out) > 0 {
			break
		}
	}
	return out
}

// builtinModels 内置模型转为 AvailableModel。
func builtinModels() []*AvailableModel {
	out := make([]*AvailableModel, 0, len(BuiltinModels))
	for _, m := range BuiltinModels {
		out = append(out, &AvailableModel{
			ID:            m.ModelID,
			Name:          m.Name,
			DisplayName:   m.DisplayName,
			Description:   m.Description,
			Capacity:      m.Capacity,
			CapacityField: 12,
			ModelNumber:   m.ModelNumber,
			Available:     true,
			Aliases:       []string{m.Name, m.DisplayName, m.ModelID},
		})
	}
	return out
}

func strAt(list []any, idx int) string {
	if idx >= 0 && idx < len(list) {
		if s, ok := list[idx].(string); ok {
			return s
		}
	}
	return ""
}
