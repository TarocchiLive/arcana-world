// Arcana 的 VITS/Kokoro 扩展；只使用 ORT 公开 API，不修改 sherpa C ABI。
#ifndef ARCANA_LOCAL_WEBGPU_H_
#define ARCANA_LOCAL_WEBGPU_H_

#include <cstdlib>
#include <cstring>
#include <stdexcept>
#include "sherpa-onnx/csrc/text-utils.h"

namespace sherpa_onnx {

class ArcanaWebGpu {
 public:
  ArcanaWebGpu(Ort::Env &env, const OfflineTtsModelConfig &config)
      : env_(env), enabled_(config.provider == "webgpu") {
    if (!enabled_) return;
    const char *path = std::getenv("ARCANA_LOCAL_WEBGPU_LIBRARY");
    if (!path || !*path) throw std::runtime_error("Local TTS WebGPU plugin path is missing");
    std::string filename(path);
    Ort::ThrowOnError(Ort::GetApi().RegisterExecutionProviderLibrary(
        env_, "arcana-webgpu", SHERPA_ONNX_TO_ORT_PATH(filename)));
  }

  ~ArcanaWebGpu() {
    // 此成员位于 env 之后、session/options 之前，析构顺序不可交换。
    if (enabled_) {
      OrtStatus *status = Ort::GetApi().UnregisterExecutionProviderLibrary(env_, "arcana-webgpu");
      if (status) Ort::GetApi().ReleaseStatus(status);
    }
  }
  ArcanaWebGpu(const ArcanaWebGpu &) = delete;
  ArcanaWebGpu &operator=(const ArcanaWebGpu &) = delete;

  static Ort::SessionOptions Options(const OfflineTtsModelConfig &config) {
    // 不经过 upstream 对未知 provider 静默回退 CPU 的分支。
    if (config.provider != "webgpu") return GetSessionOptions(config);
    auto cpu = config;
    cpu.provider = "cpu";
    return GetSessionOptions(cpu);
  }

  void Configure(Ort::SessionOptions &options) {
    if (enabled_) {
      const auto &api = Ort::GetApi();
      const OrtEpDevice *const *devices = nullptr;
      size_t count = 0;
      Ort::ThrowOnError(api.GetEpDevices(env_, &devices, &count));
      const OrtEpDevice *selected = nullptr;
      for (size_t i = 0; i < count; ++i) {
        if (std::strcmp(api.EpDevice_EpName(devices[i]), "WebGpuExecutionProvider") == 0 &&
            api.HardwareDevice_Type(api.EpDevice_Device(devices[i])) == OrtHardwareDeviceType_GPU) {
          selected = devices[i];
          break;
        }
      }
      if (!selected) throw std::runtime_error("Local TTS WebGPU: no GPU adapter available (CPU fallback is disabled)");
      // EP 0.4.0 在 Kokoro 常量权重预打包阶段发生原生崩溃；此项作用于所有 WebGPU
      // 会话（含 Melo、Piper），GPU 算子不受影响，显式 CPU 后端保留预打包。
      options.AddConfigEntry("session.disable_prepacking", "1");
      Ort::ThrowOnError(api.SessionOptionsAppendExecutionProvider_V2(
          options, env_, &selected, 1, nullptr, nullptr, 0));
      SHERPA_ONNX_LOGE("Local TTS: selected WebGpuExecutionProvider GPU device; unsupported operators may run on CPU");
    }
    // 仅显式诊断时写 ORT profile；可检查事件 args.provider 的真实算子归属。
    const char *profile = std::getenv("ARCANA_LOCAL_ORT_PROFILE");
    if (profile && *profile) {
      std::string filename(profile);
      options.EnableProfiling(SHERPA_ONNX_TO_ORT_PATH(filename));
    }
  }

 private:
  Ort::Env &env_;
  bool enabled_;
};

}  // namespace sherpa_onnx
#endif
