package submission

// StepKind identifies the kind of input expected at a given step.
type StepKind string

const (
	StepText       StepKind = "text"
	StepShortText  StepKind = "short_text"
	StepImage      StepKind = "image"
	StepImages     StepKind = "images"
	StepFinal      StepKind = "final"
)

// Step describes one prompt in the submission flow.
type Step struct {
	Key       string   // payload key + identifier in URLs / buttons
	Title     string   // Chinese label shown to user / admin
	Required  bool
	Kind      StepKind
	Prompt    string // long form prompt sent in TG messages
	Example   string // optional example
	AssetRole string // for image/images steps
}

// Steps returns the canonical ordered list, mirrored from template.md.
func Steps() []Step {
	return []Step{
		{Key: "entry_id", Title: "条目 ID", Required: true, Kind: StepShortText,
			Prompt:  "请输入条目 ID。\n它会成为页面网址，例如 /memorial/example_id。",
			Example: "推荐写英文、数字或下划线，比如 LCG_Akiball、unknown_wuhan_2022。",
		},
		{Key: "display_name", Title: "展示名", Required: true, Kind: StepShortText,
			Prompt:  "请输入展示名。\n这是页面的标题，可以是真名、网名或常用 ID。",
			Example: "例：Akiball、玖月折耳猫、无名逝者",
		},
		{Key: "avatar", Title: "头像", Required: true, Kind: StepImage, AssetRole: "avatar",
			Prompt: "请点击下方「📸 在网页上传图片」按钮上传头像。\n上传完成后点击「下一步」继续。",
		},
		{Key: "description", Title: "一句话简介", Required: true, Kind: StepShortText,
			Prompt:  "请输入一句话简介。\n会出现在列表卡片和详情页顶部，建议 20-60 字。",
			Example: "例：一个温柔、热爱游戏和做饭的跨性别女孩。",
		},
		{Key: "location", Title: "地区", Required: true, Kind: StepShortText,
			Prompt:  "请填写 ta 生活、常住、出生或主要被联系到的地区。",
			Example: "例：广东深圳、澳大利亚墨尔本、地区未公开。",
		},
		{Key: "birth_date", Title: "出生日期", Required: true, Kind: StepShortText,
			Prompt: "请填写出生日期 (YYYY-MM-DD / YYYY-MM / YYYY)，不公开请写“出生日期未公开”。",
		},
		{Key: "death_date", Title: "逝世日期", Required: true, Kind: StepShortText,
			Prompt: "请填写逝世日期 (YYYY-MM-DD / YYYY-MM / YYYY)。不确定可以写 unknown 或“逝世日期未公开”。",
		},
		{Key: "alias", Title: "昵称", Kind: StepShortText, Prompt: "可选：ta 的昵称、常用 ID、朋友常叫的名字。"},
		{Key: "age", Title: "年龄", Kind: StepShortText, Prompt: "可选：逝世时年龄。"},
		{Key: "identity", Title: "身份表述", Kind: StepShortText, Prompt: "可选：跨性别女性 / 非二元 / 性别多元 / 友跨人士…仅在公开且有依据时填写。"},
		{Key: "pronouns", Title: "代词", Kind: StepShortText, Prompt: "可选：她 / 他 / ta / they 等。"},
		{Key: "content_warnings", Title: "内容提醒", Kind: StepText,
			Prompt: "请简述正文涉及的创伤内容（自杀、家暴、精神健康等）。如无明显内容提醒，可写：无明显内容提醒。",
		},
		{Key: "intro", Title: "简介", Kind: StepText,
			Prompt: "正文 · 简介：写 ta 是谁。常用名字、性格、爱好、给朋友留下的印象。",
		},
		{Key: "intro_images", Title: "简介图片", Kind: StepImages, AssetRole: "intro",
			Prompt: "可选：在网页里上传插在简介附近的图片。完成后点击「下一步」。",
		},
		{Key: "life", Title: "生平与记忆", Kind: StepText,
			Prompt: "正文 · 生平与记忆：具体的人生片段、爱好、作品、社群经历。",
		},
		{Key: "life_images", Title: "生平与记忆图片", Kind: StepImages, AssetRole: "life",
			Prompt: "可选：在网页里上传插在生平与记忆附近的图片。完成后点击「下一步」。",
		},
		{Key: "death", Title: "离世", Kind: StepText,
			Prompt: "正文 · 离世：仅写公开且适合发布的离世信息。不写未确认传闻和过度死亡细节。",
		},
		{Key: "remembrance", Title: "念想", Kind: StepText,
			Prompt: "正文 · 念想：晚安、谢谢、对不起、我记得你、愿你不再痛苦。这是“活着的人想留给 ta 的话”。",
		},
		{Key: "remembrance_images", Title: "念想图片", Kind: StepImages, AssetRole: "remembrance",
			Prompt: "可选：在网页里上传插在念想附近的图片。完成后点击「下一步」。",
		},
		{Key: "links", Title: "公开链接", Kind: StepText,
			Prompt: "可选：每行一个公开链接，格式 “名称: https://...”。例如 twitter: https://...",
		},
		{Key: "works", Title: "作品", Kind: StepText,
			Prompt: "可选：每行一项 ta 的文章、音乐、视频、绘画、游戏、代码等。",
		},
		{Key: "sources", Title: "资料来源", Kind: StepText,
			Prompt: "可选：用于维护者核对事实，可写讣告、朋友说明、公开内容链接。",
		},
		{Key: "custom", Title: "自选附加项", Kind: StepText,
			Prompt: "可选：投稿人觉得重要、上面没覆盖的内容。每行一条。",
		},
		{Key: "submitter_contact", Title: "投稿人联系方式", Required: true, Kind: StepShortText,
			Prompt: "请填写投稿人联系方式（邮箱 / Telegram / 其他）。仅供维护者核对，不公开。",
		},
		{Key: "review", Title: "提交审核", Kind: StepFinal,
			Prompt: "全部内容已收集完成。请确认无误后点击 “✅ 提交审核”。",
		},
	}
}

// FindStep returns the step with the given key.
func FindStep(key string) (Step, bool) {
	for _, s := range Steps() {
		if s.Key == key {
			return s, true
		}
	}
	return Step{}, false
}

// NextStep returns the next step (after `key`); returns final step if exhausted.
func NextStep(key string) Step {
	steps := Steps()
	if key == "" {
		return steps[0]
	}
	for i, s := range steps {
		if s.Key == key && i+1 < len(steps) {
			return steps[i+1]
		}
	}
	return steps[len(steps)-1]
}

// PrevStep returns the step before `key`; returns first step if at start.
func PrevStep(key string) Step {
	steps := Steps()
	for i, s := range steps {
		if s.Key == key && i > 0 {
			return steps[i-1]
		}
	}
	return steps[0]
}
