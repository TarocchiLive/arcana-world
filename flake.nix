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
          vendorHash = "sha256-brywIEyno/f0k/fSJxcPr7eB/W7ZfraO0kwt3Zgt1wQ=";
        in
        rec {
          default = arcana-world-desktop;
          arcana-world = pkgs.buildGoModule rec {
            pname = "arcana-world";
            version = "0.4.0";

            src = pkgs.lib.fileset.toSource {
              root = ./.;
              fileset = pkgs.lib.fileset.unions [
                ./go.mod
                ./go.sum
                ./cmd
                ./internal
                ./third_party
                ./LICENSE
              ];
            };

            inherit vendorHash;
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
          arcana-world-tts = pkgs.buildGoModule {
            pname = "arcana-world-tts";
            inherit (arcana-world) version src;
            inherit vendorHash;
            subPackages = [ "cmd/arcana-world-tts" ];
            env.CGO_ENABLED = if pkgs.stdenv.isLinux then "1" else "0";
            tags = [ "tts_audio" ];
            nativeBuildInputs = pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.pkg-config ];
            buildInputs = pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.alsa-lib ];
            ldflags = [ "-s" "-w" ];
            checkPhase = ''
              runHook preCheck
              go test -tags tts_audio ./internal/tts/audio ./cmd/arcana-world-tts
              runHook postCheck
            '';
            meta = arcana-world.meta // {
              description = "Edge speech synthesis and native MP3 playback helper";
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
            install -Dm755 ${arcana-world-tts}/bin/arcana-world-tts "$out/bin/libexec/arcana-world-tts"
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
        in
        rec {
          default = pkgs.mkShell {
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
          };
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
          arcana-world-tts = pkgs.mkShell {
            packages = [ pkgs.go ] ++ pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.pkg-config ];
            buildInputs = pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.alsa-lib ];
            CGO_ENABLED = if pkgs.stdenv.isLinux then "1" else "0";
            GOFLAGS = "-mod=readonly";
          };
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
