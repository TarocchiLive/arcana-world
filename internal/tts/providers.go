package tts

import (
	"errors"
	"math"
	"strconv"

	"arcana-world/internal/i18n"
	"arcana-world/internal/tts/models"
)

// Option 是固定选项字段的一个可选值。
type Option struct {
	ID    string
	Label i18n.Key
}

// Field 描述一个供应商参数；Options 非空时只能从中选择，Secret 字段保存在凭据后端。
type Field struct {
	ID      string
	Label   i18n.Key
	Default string
	Secret  bool
	Options []Option
}

// Provider 将界面描述、轻量校验与实例创建集中在同一个注册项。
type Provider struct {
	ID            string
	Name          i18n.Key
	SettingsTitle i18n.Key
	Fields        []Field
	Model         bool // 需要先安装本地语音模型。
	Validate      func(map[string]string) error
	Factory       func(values map[string]string, appDir, proxy string) (Synthesizer, error)
}

var chineseVoices = []Option{
	{"zh-CN-XiaoxiaoNeural", i18n.OutputVoiceXiaoxiao},
	{"zh-CN-XiaoyiNeural", i18n.OutputVoiceXiaoyi},
	{"zh-CN-YunxiNeural", i18n.OutputVoiceYunxi},
	{"zh-CN-YunjianNeural", i18n.OutputVoiceYunjian},
	{"zh-CN-YunxiaNeural", i18n.OutputVoiceYunxia},
	{"zh-CN-YunyangNeural", i18n.OutputVoiceYunyang},
	{"zh-HK-HiuGaaiNeural", i18n.OutputVoiceHiuGaai},
	{"zh-HK-WanLungNeural", i18n.OutputVoiceWanLung},
	{"zh-TW-HsiaoChenNeural", i18n.OutputVoiceHsiaoChen},
	{"zh-TW-YunJheNeural", i18n.OutputVoiceYunJhe},
}

var providers = []Provider{
	{
		ID: "edge", Name: i18n.TTSEdge, SettingsTitle: i18n.TTSEdgeSettings,
		Fields: []Field{{ID: "voice", Label: i18n.OutputVoice, Default: defaultVoice, Options: chineseVoices}},
		Validate: func(values map[string]string) error {
			for _, voice := range chineseVoices {
				if values["voice"] == voice.ID {
					return nil
				}
			}
			return errors.New(i18n.T(i18n.OutputVoiceUnsupported))
		},
		Factory: func(values map[string]string, _, proxy string) (Synthesizer, error) {
			return newEdge(values["voice"], proxy)
		},
	},
	localProvider("melo", i18n.TTSMelo, i18n.TTSMeloSettings, nil, ""),
	localProvider("kokoro", i18n.TTSKokoro, i18n.TTSKokoroSettings, kokoroVoices(), "3"),
	localProvider("piper", i18n.TTSPiper, i18n.TTSPiperSettings, []Option{{"0", i18n.TTSPiperVoice}}, "0"),
	{
		ID: "qwen", Name: i18n.TTSQwen, SettingsTitle: i18n.TTSQwenSettings,
		Fields: []Field{
			{ID: "base_url", Label: i18n.TTSBaseURL, Default: "https://dashscope.aliyuncs.com/api/v1"},
			{ID: "api_key", Label: i18n.TTSAPIKey, Secret: true},
			{ID: "model", Label: i18n.TTSCloudModel, Default: qwenDefaultModel},
			{ID: "voice", Label: i18n.OutputVoice, Default: qwenDefaultVoice},
			{ID: "language", Label: i18n.TTSLanguage, Default: qwenDefaultLanguage},
		},
		Validate: validateQwen,
		Factory: func(values map[string]string, _, proxy string) (Synthesizer, error) {
			return newQwen(values, proxy)
		},
	},
	{
		ID: "doubao", Name: i18n.TTSDoubao, SettingsTitle: i18n.TTSDoubaoSettings,
		Fields: []Field{
			{ID: "base_url", Label: i18n.TTSBaseURL, Default: "https://openspeech.bytedance.com"},
			{ID: "api_key", Label: i18n.TTSAPIKey, Secret: true},
			{ID: "resource_id", Label: i18n.TTSResourceID, Default: doubaoDefaultResource},
			{ID: "voice", Label: i18n.OutputVoice, Default: doubaoDefaultVoice},
		},
		Validate: validateDoubao,
		Factory: func(values map[string]string, _, proxy string) (Synthesizer, error) {
			return newDoubao(values, proxy)
		},
	},
}

func localProvider(id string, name, title i18n.Key, voices []Option, defaultVoice string) Provider {
	fields := []Field{
		{ID: "backend", Label: i18n.TTSMeloBackend, Default: DefaultLocalBackend, Options: []Option{{"webgpu", i18n.TTSMeloBackendWebGPU}, {"cpu", i18n.TTSMeloBackendCPU}}},
		{ID: "threads", Label: i18n.TTSThreads, Default: strconv.Itoa(DefaultLocalThreads)},
		{ID: "speed", Label: i18n.TTSSpeed, Default: "1"},
	}
	if len(voices) != 0 {
		fields = append(fields, Field{ID: "voice", Label: i18n.OutputVoice, Default: defaultVoice, Options: voices})
	}
	return Provider{
		ID: id, Name: name, SettingsTitle: title, Model: true, Fields: fields,
		Validate: func(values map[string]string) error {
			if values["backend"] != "webgpu" && values["backend"] != "cpu" {
				return errors.New(i18n.T(i18n.OutputMeloBackendInvalid))
			}
			threads, err := strconv.Atoi(values["threads"])
			if err != nil || threads < 1 || threads > 32 {
				return errors.New(i18n.T(i18n.OutputMeloThreadsInvalid))
			}
			speed, err := strconv.ParseFloat(values["speed"], 64)
			if err != nil || math.IsNaN(speed) || math.IsInf(speed, 0) || speed < 0.5 || speed > 2 {
				return errors.New(i18n.T(i18n.OutputMeloSpeedInvalid))
			}
			if len(voices) != 0 {
				for _, voice := range voices {
					if values["voice"] == voice.ID {
						return nil
					}
				}
				return errors.New(i18n.T(i18n.OutputVoiceUnsupported))
			}
			return nil
		},
		Factory: func(values map[string]string, appDir, _ string) (Synthesizer, error) {
			threads, _ := strconv.Atoi(values["threads"])
			speed, _ := strconv.ParseFloat(values["speed"], 64)
			speaker, _ := strconv.Atoi(values["voice"])
			return NewLocal(LocalOptions{Provider: id, ModelDir: models.DefaultDir(appDir, id), Backend: values["backend"], Threads: threads, Speaker: speaker, Speed: speed})
		},
	}
}

// 音色顺序对应上游 v1.1 voices.bin，不能按音色名称中的编号推算。
// https://k2-fsa.github.io/sherpa/onnx/tts/all/Chinese-English/kokoro-multi-lang-v1_1.html
func kokoroVoices() []Option {
	voices := make([]Option, 103)
	for id := range voices {
		voices[id] = Option{strconv.Itoa(id), i18n.Key("tts.voice.kokoro." + strconv.Itoa(id))}
	}
	return voices
}

// Providers 返回独立的描述，调用方不能修改注册表。
func Providers() []Provider {
	result := make([]Provider, len(providers))
	for i, provider := range providers {
		result[i] = cloneProvider(provider)
	}
	return result
}

func cloneProvider(provider Provider) Provider {
	provider.Fields = append([]Field(nil), provider.Fields...)
	for i := range provider.Fields {
		provider.Fields[i].Options = append([]Option(nil), provider.Fields[i].Options...)
	}
	return provider
}

func ProviderByID(id string) (Provider, bool) {
	provider, ok := providerByID(id)
	if !ok {
		return Provider{}, false
	}
	return cloneProvider(provider), true
}

func providerByID(id string) (Provider, bool) {
	for _, provider := range providers {
		if provider.ID == id {
			return provider, true
		}
	}
	return Provider{}, false
}

func NewSynthesizer(settings Settings, appDir, proxy string) (Synthesizer, error) {
	settings, err := settings.Normalize()
	if err != nil {
		return nil, err
	}
	provider, _ := providerByID(settings.Provider)
	return provider.Factory(settings.Providers[settings.Provider], appDir, proxy)
}
