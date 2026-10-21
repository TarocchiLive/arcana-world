// 固定资源清单在发布时从实际归档计算；运行时不接受远端配置提供的摘要。
package models

// Spec 描述应用共享的固定版本模型；内部清单不暴露给调用方修改。
type Spec struct {
	Name, ArchiveURL                 string
	ArchiveSize                      int64
	SampleRate, Speakers             int
	extension, prefix, archiveSHA256 string
	assets                           map[string]asset
}

// SpecFor 返回已审核的模型规格；未知标识不回退到其他模型。
func SpecFor(provider string) (Spec, bool) {
	switch provider {
	case "melo":
		return Spec{
			Name:        "melo-zh-en",
			ArchiveURL:  "https://dl.hdslb.com/bili/bililive/win/sherpa-onnx-shared-resources/27d5522a87dd49aa7f34f01fd5d9f9ee.zip",
			ArchiveSize: 164436780, SampleRate: 44100, Speakers: 1,
			extension: ".zip", archiveSHA256: "6c9e380a10c6bcbfdfab7c39f5ff8fc4c566bdbcd65f54c2b8ac8acae30e8573",
			assets: meloAssets,
		}, true
	case "kokoro":
		return kokoroSpec, true
	case "piper":
		return piperSpec, true
	default:
		return Spec{}, false
	}
}

type asset struct {
	size   int64
	sha256 string
}

var meloAssets = map[string]asset{
	"LICENSE":                           {1071, "7018bd16b0dca61f76e7a7c901c3e97b64a0636b9574ae8be36856cade9e11a1"},
	"README.md":                         {264, "b74367928bc4a814a38456f8b36d10eb82ca93c58c5856418fea6eded8f25f8a"},
	"config.json":                       {400, "58ffc5503bee863595281b0e3fb3596ad99fcf8c05a50fa5886b39b3cf4f4440"},
	"date.fst":                          {59154, "eb8aa079ae3cb81d8f4404992f39d61a0cb990947512b5b8d1e54d1f6980e718"},
	"dict/README.md":                    {714, "324dbf050d1516b8eb938b02d19a3f90cc67c97e9fc002033595c54c0cd5cd9e"},
	"dict/hmm_model.utf8":               {519773, "c2781984c4c6be66862ff1bc9aace264256bcd174cd900d7f3725debe6c71255"},
	"dict/idf.utf8":                     {6257543, "d91455238e015eb0af6dd9539d55d5c4f5059513724545e1b057ed696438c663"},
	"dict/jieba.dict.utf8":              {5420186, "59578ca2e4bf8b0ca9de879d6484fe70453cb7d46dbc430d15bf943541d5a330"},
	"dict/pos_dict/char_state_tab.utf8": {333792, "d5e2f71cfb65aeeda9899b0268aa3fb2ce917813c5b5f89d4393f5ca39e030c0"},
	"dict/pos_dict/prob_emit.utf8":      {1687852, "5f4dca59263df0f2a1c3cf706daf0222619a979315a94b3f05fd750951a01ff0"},
	"dict/pos_dict/prob_start.utf8":     {4606, "46446f353d293b934d6bacb4f8e16adc2535ff2e72bc0712de6846c33d732993"},
	"dict/pos_dict/prob_trans.utf8":     {129381, "5c051d34df75d414bf41d8df2b080961e3760b900e8f13e5f891df3cb4ea7e03"},
	"dict/stop_words.utf8":              {10508, "01190ee2a5199f1eafaa90a4d192420d3389ff17a81772a303c8912a1882a485"},
	"dict/user.dict.utf8":               {53, "88b5d41305b7fc121dbf0c762b708a3c863fa4ac0769316b1819f44cd452d22b"},
	"lexicon.txt":                       {7033499, "d5338968f710ab4e8a32c1bd47b8957c434ff5aee28c77601e6c4d7168852fee"},
	"model.onnx":                        {170429550, "bf30582eb1b012250a35b1a4a80e7dfbcf8485e7bb9de0d95efbbeef0e4ad86d"},
	"new_heteronym.fst":                 {21974, "ca14b2127e27baa571664e4bb791e143e7425f56a6bc29db08d74f97e6aa4e29"},
	"number.fst":                        {64482, "743f402181fcfebf76cc2f0546b71fa26476e626fbe4e460fb7b4c3a7a8bd5bd"},
	"phone.fst":                         {88630, "1ac2b6fa56b1442320c4de7db08353bab8963a2b57f365eebcdd3a2d3562f8d7"},
	"tokens.txt":                        {767, "be76fa98ab761477cfbe04fc0c7ccb66525c6611dd0917fbb023257b677066b1"},
}
