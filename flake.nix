{
  description = "A lightweight TUI alternative to bilibili LiveHime";

  # 26.05 仍支持 Intel macOS，具体修订和内容哈希由 flake.lock 固定。
  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-26.05";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = nixpkgs.lib.genAttrs systems;
    in
    {
      packages = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
          vendorHash = "sha256-pczP2+cgD4jM1R+HnSOEL9PqIaquqJ8yU9LT3PIhtlI=";
        in
        rec {
          default = arcana-world-desktop;
          arcana-world = pkgs.buildGoModule rec {
            pname = "arcana-world";
            version = "0.4.3";

            src = pkgs.lib.fileset.toSource {
              root = ./.;
              fileset = pkgs.lib.fileset.unions [
                ./go.mod
                ./go.sum
                ./cmd
                ./internal
                ./scripts/build-tts-native.py
                ./scripts/tts-native
                ./LICENSE
              ];
            };

            inherit vendorHash;
            proxyVendor = true;
            subPackages = [ "cmd/arcana-world" ];
            env.CGO_ENABLED = "0";
            ldflags = [
              "-s"
              "-w"
              "-X main.version=${version}"
            ];

            checkPhase = ''
              runHook preCheck
              go test ./...
              runHook postCheck
            '';

            postInstall = ''
              install -Dm644 LICENSE "$out/share/licenses/arcana-world/LICENSE"
            '';

            meta = {
              description = "A lightweight TUI alternative to bilibili LiveHime";
              homepage = "https://github.com/TarocchiLive/arcana-world";
              license = pkgs.lib.licenses.gpl3Only;
              mainProgram = "arcana-world";
              platforms = systems;
            };
          };
          arcana-world-overlay-native = pkgs.buildGoModule {
            pname = "arcana-world-overlay-native";
            inherit (arcana-world) version src;
            inherit vendorHash;
            proxyVendor = true;
            subPackages = [ "cmd/arcana-world-overlay" ];
            env.CGO_ENABLED = "1";
            tags = pkgs.lib.optionals pkgs.stdenv.isLinux [ "wayland" ];
            nativeBuildInputs = pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.pkg-config ];
            buildInputs = pkgs.lib.optionals pkgs.stdenv.isLinux [
              pkgs.wayland
              pkgs.pango
              pkgs.cairo
            ];
            ldflags = [ "-s" "-w" ];
            postInstall = ''
              mkdir -p "$out/libexec"
              mv "$out/bin/arcana-world-overlay" "$out/libexec/arcana-world-overlay-native"
              rmdir "$out/bin"
              install -Dm644 LICENSE "$out/share/licenses/arcana-world-overlay/LICENSE"
            '';
            meta = {
              description = "Native click-through desktop text overlay";
              homepage = "https://github.com/TarocchiLive/arcana-world";
              license = pkgs.lib.licenses.gpl3Only;
              mainProgram = "arcana-world";
              platforms = systems;
            };
          };
          awqt = pkgs.stdenv.mkDerivation {
            pname = "arcana-world-overlay-renderer";
            inherit (arcana-world) version src;
            sourceRoot = "source/internal/overlay/qt/cpp";
            nativeBuildInputs = [ pkgs.cmake pkgs.pkg-config ];
            buildInputs = [ pkgs.qt6.qtbase ] ++ pkgs.lib.optionals pkgs.stdenv.isLinux [
              pkgs.qt6.qtwayland pkgs.kdePackages.layer-shell-qt pkgs.wayland
            ];
            dontWrapQtApps = true;
          };
          arcana-world-overlay-qt =
            assert pkgs.lib.assertMsg
              (!pkgs.stdenv.isDarwin || pkgs.lib.versionAtLeast pkgs.qt6.qtbase.version "6.11")
              "Elegant overlay on macOS requires Qt >= 6.11; use arcana-world-desktop-native with older nixpkgs.";
            pkgs.buildGoModule {
              pname = "arcana-world-overlay-qt";
              inherit (arcana-world) version src;
              inherit vendorHash;
              proxyVendor = true;
              subPackages = [ "cmd/arcana-world-overlay" ];
              env.CGO_ENABLED = "1";
              tags = [ "qt" ];
              nativeBuildInputs = [ pkgs.qt6.wrapQtAppsHook ];
              # Go 依赖下载阶段不继承 Qt 库，包装 hook 仅用于最终程序。
              overrideModAttrs = _: previous: {
                nativeBuildInputs = builtins.filter
                  (input: input != pkgs.qt6.wrapQtAppsHook)
                  previous.nativeBuildInputs;
              };
              buildInputs = [ awqt pkgs.qt6.qtbase ] ++ pkgs.lib.optionals pkgs.stdenv.isLinux [
                pkgs.qt6.qtwayland pkgs.kdePackages.layer-shell-qt
              ];
              preBuild = ''
                export CGO_LDFLAGS="-L${awqt} -Wl,-rpath,${awqt}"
              '';
              postInstall = ''
                mkdir -p "$out/libexec"
                mv "$out/bin/arcana-world-overlay" "$out/libexec/arcana-world-overlay-qt"
                rmdir "$out/bin"
                ${pkgs.lib.optionalString pkgs.stdenv.isDarwin ''
                  app="$out/libexec/Arcana World Elegant Overlay.app"
                  mkdir -p "$app/Contents/MacOS"
                  mv "$out/libexec/arcana-world-overlay-qt" "$app/Contents/MacOS/arcana-world-overlay-qt"
                  cp internal/overlay/qt/Info.plist "$app/Contents/Info.plist"
                  wrapQtApp "$app/Contents/MacOS/arcana-world-overlay-qt"
                  substituteInPlace "$app/Contents/Info.plist" \
                    --replace-fail '<string>arcana-world-overlay-qt</string>' '<string>.arcana-world-overlay-qt-wrapped</string>'
                  ln -s "Arcana World Elegant Overlay.app/Contents/MacOS/arcana-world-overlay-qt" "$out/libexec/arcana-world-overlay-qt"
                ''}
                ${pkgs.lib.optionalString (!pkgs.stdenv.isDarwin) ''
                  wrapQtApp "$out/libexec/arcana-world-overlay-qt"
                ''}
              '';
              dontWrapQtApps = true;
              meta = arcana-world.meta // { description = "Elegant desktop overlay helper"; };
            };
          arcana-world-tts = let
            sources = builtins.fromJSON (builtins.readFile ./scripts/tts-native/sources.json);
            gpuSupported = !pkgs.stdenv.isDarwin || pkgs.stdenv.hostPlatform.isAarch64;
            goos = if pkgs.stdenv.isDarwin then "darwin" else "linux";
            goarch = if pkgs.stdenv.hostPlatform.isAarch64 then "arm64" else "amd64";
            ortName = if pkgs.stdenv.isDarwin then "onnxruntime-osx-arm64"
              else if pkgs.stdenv.hostPlatform.isAarch64 then "onnxruntime-linux-aarch64"
              else "onnxruntime-linux-x86_64";
            fetch = spec: pkgs.fetchurl { inherit (spec) url sha256; };
            unpack = name: spec: pkgs.runCommand "tts-source-${name}" {
              nativeBuildInputs = [ pkgs.unzip ];
            } ''
              mkdir unpack
              cd unpack
              ${if pkgs.lib.hasSuffix ".zip" spec.url then
                "unzip -q ${fetch spec}" else "tar -xf ${fetch spec}"}
              roots=(*)
              test "''${#roots[@]}" -eq 1
              cp -R "''${roots[0]}" "$out"
            '';
            dependencies = pkgs.lib.filterAttrs (name: _: 
              !(pkgs.lib.hasPrefix "onnxruntime-" name) || name == ortName
            ) sources.dependencies;
          in pkgs.buildGoModule {
            pname = "arcana-world-tts";
            inherit (arcana-world) version src;
            inherit vendorHash;
            # 原生模块的 lib/ 目录不是 Go package，不能使用 go mod vendor。
            proxyVendor = true;
            subPackages = [ "cmd/arcana-world-tts" ];
            env.CGO_ENABLED = "1";
            tags = [ "tts" ];
            nativeBuildInputs = [ pkgs.cmake pkgs.ninja pkgs.python3 ] ++ pkgs.lib.optionals pkgs.stdenv.isLinux [
              pkgs.pkg-config pkgs.autoPatchelfHook
            ];
            buildInputs = pkgs.lib.optionals pkgs.stdenv.isLinux [
              pkgs.alsa-lib pkgs.stdenv.cc.cc.lib
            ];
            # Vulkan 由插件动态加载，常规 ELF 依赖扫描无法发现。
            runtimeDependencies = pkgs.lib.optionals pkgs.stdenv.isLinux [ (pkgs.lib.getLib pkgs.vulkan-loader) ];
            dontUseCmakeConfigure = true;
            # espeak-ng 以 -Wno-format 编译，与 Nix 默认的 -Werror=format-security 冲突。
            hardeningDisable = [ "format" ];
            # 依赖下载只需 Go 模块；默认会继承 preBuild，在依赖派生中重复构建原生库。
            overrideModAttrs = _: { preBuild = ""; };
            preBuild = pkgs.lib.optionalString gpuSupported ''
              mkdir -p "$TMPDIR/tts-deps" "$TMPDIR/tts-stage"
              ${pkgs.lib.concatStringsSep "\n" (pkgs.lib.mapAttrsToList (name: spec: ''
                cp -R ${unpack name spec} "$TMPDIR/tts-deps/${spec.cmake_name}"
              '') dependencies)}
              chmod -R u+w "$TMPDIR/tts-deps"
              TTS_SHERPA_SOURCE=${unpack "sherpa" sources.sherpa} \
              TTS_NATIVE_DEPS_DIR="$TMPDIR/tts-deps" \
              TTS_WEBGPU_PACKAGE=${fetch sources.webgpu} \
              BUILD_JOBS="$NIX_BUILD_CORES" \
                python3 scripts/build-tts-native.py "$TMPDIR/tts-build" "$TMPDIR/tts-stage" ${goos} ${goarch}
            '';
            ldflags = [ "-s" "-w" ];
            checkPhase = ''
              runHook preCheck
              go test -tags tts ./internal/tts/... ./cmd/arcana-world-tts
              runHook postCheck
            '';
            postInstall = let
              platform = if pkgs.stdenv.isDarwin then "macos" else "linux";
              arch = if pkgs.stdenv.hostPlatform.isAarch64 then "aarch64" else "x86_64";
              triple = arch + (if pkgs.stdenv.isDarwin then "-apple-darwin" else "-unknown-linux-gnu");
              ortLicense = pkgs.fetchurl {
                url = "https://raw.githubusercontent.com/microsoft/onnxruntime/v1.28.2/LICENSE";
                hash = "sha256-LwfHJ1Gu2ZeQuKSGnPIxHfhahgsi3tBfoigDWHpIkiw=";
              };
              ortNotices = pkgs.fetchurl {
                url = "https://raw.githubusercontent.com/microsoft/onnxruntime/v1.28.2/ThirdPartyNotices.txt";
                hash = "sha256-Dge5XzqNYjADdwfFxKK1VNEsTLZzaWaawlVjVSj/zuI=";
              };
            in ''
              module="$(go env GOMODCACHE)/github.com/k2-fsa/sherpa-onnx-go-${platform}@v1.13.8"
              install -Dm644 "$module/LICENSE" "$out/share/licenses/tts/sherpa-onnx-LICENSE"
              install -Dm644 ${ortLicense} "$out/share/licenses/tts/onnxruntime-LICENSE"
              install -Dm644 ${ortNotices} "$out/share/licenses/tts/onnxruntime-ThirdPartyNotices.txt"
              ${pkgs.lib.optionalString gpuSupported ''
                cp -R "$TMPDIR/tts-stage/." "$out/bin/"
                cp -R "$out/bin/licenses/tts/." "$out/share/licenses/tts/"
                rm -rf "$out/bin/licenses"
              ''}
              ${pkgs.lib.optionalString pkgs.stdenv.isLinux ''
                chmod u+w "$out/bin/"*.so
                # 去掉构建目录和模块缓存的 RPATH；系统库由 autoPatchelfHook 随后补齐。
                for binary in "$out/bin/arcana-world-tts" "$out/bin/"*.so; do
                  patchelf --set-rpath '$ORIGIN' "$binary"
                done
              ''}
              ${pkgs.lib.optionalString pkgs.stdenv.isDarwin ''
                ${pkgs.lib.optionalString (!gpuSupported) ''
                  cp "$module/lib/${triple}/libsherpa-onnx-c-api.dylib" "$out/bin/"
                  cp "$module/lib/${triple}/libonnxruntime.dylib" "$out/bin/"
                ''}
                chmod u+w "$out/bin/"*.dylib
                for binary in "$out/bin/arcana-world-tts" "$out/bin/"*.dylib; do
                  install_name_tool -change @rpath/libsherpa-onnx-c-api.dylib @loader_path/libsherpa-onnx-c-api.dylib "$binary"
                  install_name_tool -change @rpath/libonnxruntime.dylib @loader_path/libonnxruntime.dylib "$binary"
                  codesign --force --sign - "$binary"
                done
              ''}
            '';
            meta = arcana-world.meta // {
              description = "Speech helper for audio playback and local MeloTTS, Kokoro, and Piper voices on GPU or CPU";
              mainProgram = "arcana-world-tts";
            };
          };
          arcana-world-desktop-native = pkgs.runCommand "arcana-world-desktop-native-${arcana-world.version}" {
            meta = arcana-world.meta // {
              description = "Arcana World with native overlay and speech helper";
            };
          } ''
            # 与 scripts/build.sh 使用的便携发行包布局保持一致。
            install -Dm755 ${arcana-world}/bin/arcana-world "$out/bin/arcana-world"
            install -Dm755 ${arcana-world-overlay-native}/libexec/arcana-world-overlay-native "$out/bin/libexec/arcana-world-overlay-native"
            mkdir -p "$out/bin/libexec" "$out/share/licenses"
            cp -R ${arcana-world-tts}/bin/. "$out/bin/libexec/"
            cp -R ${arcana-world-tts}/share/licenses/. "$out/share/licenses/"
            mkdir -p "$out/bin/lib"
            ln -s ${pkgs.lib.getLib pkgs.srt}/lib/libsrt.${if pkgs.stdenv.isDarwin then "dylib" else "so"} \
              "$out/bin/lib/libsrt.${if pkgs.stdenv.isDarwin then "dylib" else "so"}"
            install -Dm644 ${arcana-world}/share/licenses/arcana-world/LICENSE \
              "$out/share/licenses/arcana-world/LICENSE"
          '';
          arcana-world-desktop = pkgs.runCommand "arcana-world-desktop-${arcana-world.version}" {
            meta = arcana-world.meta // { description = "Arcana World with native and Elegant overlays"; };
          } ''
            mkdir -p "$out/bin/libexec"
            cp -R ${arcana-world-desktop-native}/bin/. "$out/bin/"
            cp -R ${arcana-world-overlay-qt}/libexec/. "$out/bin/libexec/"
            mkdir -p "$out/share"
            cp -R ${arcana-world-desktop-native}/share/. "$out/share/"
          '';
        }
      );

      devShells = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
          # 构建语音原生库的 shell 共用：espeak-ng 以 -Wno-format 编译，与 Nix 默认的
          # -Werror=format-security 冲突；预编译依赖包面向 Ubuntu/macOS/Windows 工具链，Nix 环境始终从源码构建。
          ttsShellEnv = {
            hardeningDisable = [ "format" ];
            NATIVE_DEPS = "source";
          };
        in
        rec {
          default = pkgs.mkShell (ttsShellEnv // {
            inputsFrom = [
              arcana-world
              arcana-world-overlay-native
              arcana-world-tts
            ];
            packages = [ pkgs.go pkgs.cmake pkgs.python3 pkgs.pkg-config pkgs.qt6.qtbase ]
              ++ pkgs.lib.optionals pkgs.stdenv.isLinux [
                pkgs.patchelf pkgs.qt6.qtwayland pkgs.kdePackages.layer-shell-qt
                pkgs.xkeyboard_config pkgs.fontconfig
              ];
            QTPATHS = "${pkgs.qt6.qtbase}/bin/qtpaths";
            QT_PLUGIN_PATH = pkgs.lib.makeSearchPath pkgs.qt6.qtbase.qtPluginPrefix (
              [ pkgs.qt6.qtbase ] ++ pkgs.lib.optionals pkgs.stdenv.isLinux [
                pkgs.qt6.qtwayland pkgs.kdePackages.layer-shell-qt
              ]
            );
            CGO_ENABLED = "1";
            GOFLAGS = "-mod=readonly";
          });
          arcana-world = pkgs.mkShell {
            packages = [ pkgs.go ];
            CGO_ENABLED = "0";
            GOFLAGS = "-mod=readonly";
          };
          arcana-world-overlay-native = pkgs.mkShell {
            packages = [ pkgs.go ] ++ pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.pkg-config pkgs.python3 pkgs.patchelf ];
            buildInputs = pkgs.lib.optionals pkgs.stdenv.isLinux [
              pkgs.wayland
              pkgs.pango
              pkgs.cairo
              pkgs.xkeyboard_config
              pkgs.fontconfig
            ];
            CGO_ENABLED = "1";
            GOFLAGS = "-mod=readonly" + pkgs.lib.optionalString pkgs.stdenv.isLinux " -tags=wayland";
          };
          arcana-world-tts = pkgs.mkShell (ttsShellEnv // {
            packages = [ pkgs.go pkgs.cmake pkgs.ninja pkgs.python3 ]
              ++ pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.pkg-config ];
            buildInputs = pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.alsa-lib ];
            CGO_ENABLED = "1";
            GOFLAGS = "-mod=readonly";
          });
        }
      );

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/arcana-world";
          meta.description = self.packages.${system}.default.meta.description;
        };
        arcana-world-desktop = {
          type = "app";
          program = "${self.packages.${system}.arcana-world-desktop}/bin/arcana-world";
          meta.description = self.packages.${system}.arcana-world-desktop.meta.description;
        };
      });

      checks = forAllSystems (system: {
        inherit (self.packages.${system}) arcana-world arcana-world-overlay-native arcana-world-tts arcana-world-desktop-native;
      } // nixpkgs.lib.optionalAttrs
        (!(nixpkgs.lib.hasSuffix "-darwin" system)
          || nixpkgs.lib.versionAtLeast (import nixpkgs { inherit system; }).qt6.qtbase.version "6.11") {
          inherit (self.packages.${system}) arcana-world-overlay-qt arcana-world-desktop;
        });
    };
}
