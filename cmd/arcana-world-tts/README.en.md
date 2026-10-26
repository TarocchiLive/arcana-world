# Arcana World speech

[中文](README.md) · **English**

Reads chat messages, gifts, and Super Chats aloud, using an online voice or a voice installed on your computer.

[Back to Arcana World](../../README.en.md)

## Quick start

1. On the TTS page, choose a voice provider and open its settings. In WebUI, go to Outputs → Voice.
2. Pick a voice. For a cloud voice, enter your API key; for a local voice, install the model.
3. Choose "Preview the current voice" (in WebUI: Preview voice) and make sure you can hear it.
4. In "Select spoken events" (in WebUI: Voice events), tick what should be read, then turn speech on.

Sound plays on the computer running Arcana World. Each provider keeps its own settings, so switching back does not lose them. Extract the whole release archive and keep the `libexec/` folder next to the main program; replace both when you upgrade.

## Which voice should I use?

| Voice | What it offers | What you need |
| --- | --- | --- |
| Edge | Free, several Chinese voices, occasionally unreachable for a while | Internet access |
| Qwen | Paid cloud voice, reliable | An Alibaba Cloud Model Studio API key |
| Doubao | Paid cloud voice, reliable, many voices | A Volcano Engine API key |
| MeloTTS | Local, Chinese and English, one female voice | About 164 MB model download |
| Kokoro v1.1 | Local, Chinese and English, 103 voices | About 365 MB model download |
| Piper | Local, English only, an American female voice | About 67 MB model download |

Local voices work offline once installed, but they use some of your computer's power while speaking, so keep an eye on it while streaming games.

## Qwen and Doubao

**Qwen**: turn on speech synthesis in [Alibaba Cloud Model Studio](https://www.alibabacloud.com/help/en/model-studio/qwen-tts-api), create an API key, and enter it in the settings. The default address is for the Chinese mainland region; for an international account, change it to your region's address. Model, voice, and language have defaults you can change.

**Doubao**: turn on speech synthesis in [Volcano Engine](https://www.volcengine.com/docs/6561/1598757), get an API key, and enter it in the settings. The default voice is Vivi 2.0. To use another voice, enter a resource ID and [voice ID](https://www.volcengine.com/docs/6561/1257544) your account has access to.

A saved API key is not shown again; to change it, just enter a new one. Restore default settings keeps your API keys, and Clear application data deletes them. If the network hiccups or the service is busy, the app retries twice on its own before showing an error.

## Local models

The first time you pick a model that isn't installed, the app asks whether to download it. You can also choose Install model in the settings. You can cancel a download at any time; it picks up where it left off next time, and the model is checked before it is installed.

Models are kept in the `models/tts/` folder next to the main program, so the program must be in a place you can write to. All data folders share these models, and neither Clear application data nor Restore default settings removes them.

### Installing a model yourself

If downloading in the app is too slow, download the model package yourself:

| Model | Download | Model folder (inside `models/tts/`) |
| --- | --- | --- |
| MeloTTS | [ZIP](https://dl.hdslb.com/bili/bililive/win/sherpa-onnx-shared-resources/27d5522a87dd49aa7f34f01fd5d9f9ee.zip) | `melo-zh-en/` |
| Kokoro v1.1 | [tar.bz2](https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/kokoro-multi-lang-v1_1.tar.bz2) | `kokoro-multi-lang-v1_1/` |
| Piper | [tar.bz2](https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/vits-piper-en_US-ljspeech-medium.tar.bz2) | `vits-piper-en_US-ljspeech-medium/` |

Then do one of the following:

- Put the downloaded archive at the archive path shown in the settings and choose Install model.
- Extract all files from the archive into the model folder shown in the settings and choose Verify model. The files must sit directly in that folder, not inside an extra subfolder.

## GPU or CPU

Local voices use the graphics card by default. If you see an error that the GPU cannot start, change Run on to CPU in the settings. You can also adjust CPU threads and speech speed there.

| System | GPU support |
| --- | --- |
| macOS | Apple silicon on macOS 14 or later; Intel Macs use the CPU |
| Windows | 64-bit (x64, ARM64); 32-bit uses the CPU |
| Linux | x64 and ARM64, with a system graphics driver supporting Vulkan; release bundles include the Vulkan loader |

On NixOS, system-configured graphics drivers are discovered automatically; no manual NVIDIA, AMD, or Intel driver-file selection is needed.

## Troubleshooting

| Problem | What to do |
| --- | --- |
| The preview is silent | Check this computer's output device, system volume, and TTS volume |
| The preview works, but nothing is read during a stream | Check the speech switch, chat listening, and selected events; only events that arrive after you turn speech on are read |
| The API key is rejected or the service is not enabled | Enter the API key again and make sure your account has speech synthesis and the chosen voice enabled |
| Cannot reach the service | Check the network and proxy; for Qwen and Doubao, also check the service address |
| Model verification fails | Install it again; when installing yourself, use the complete package from the table above and make sure the files are not inside an extra subfolder |
| The folder cannot be written | Move the whole program to a writable location, then install the model |
| The speech component is missing | Extract the full release archive again and keep the `libexec/` folder |

To change the wording that is read aloud, see "Customize overlay and TTS formats" in terminal help.

## Building from source

`make desktop` also builds the speech program. It needs Python 3, CMake 3.24 or later, Ninja, and a C/C++ compiler; Linux also needs the ALSA development package. Use `make tts` to build only the speech program. See the full per-platform requirements with:

```sh
bash scripts/build.sh --help
```

Keep the licenses included in the model packages. See the [main project's acknowledgments](../../README.en.md#acknowledgments) for related projects.
