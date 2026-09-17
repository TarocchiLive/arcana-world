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
          vendorHash = "sha256-bvZzhgRceIq/YNgDwX7FNNE+tdI6BG/cwrNmzqBLFKk=";
        in
        rec {
          default = arcana-world-desktop;
          arcana-world = pkgs.buildGoModule rec {
            pname = "arcana-world";
            version = "unstable-${self.shortRev or "local"}";

            src = pkgs.lib.fileset.toSource {
              root = ./.;
              fileset = pkgs.lib.fileset.unions [
                ./go.mod
                ./go.sum
                ./cmd
                ./internal
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
          arcana-overlay = pkgs.buildGoModule {
            pname = "arcana-overlay";
            inherit (arcana-world) version src;
            inherit vendorHash;
            subPackages = [ "cmd/arcana-overlay" ];
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
              install -Dm644 LICENSE "$out/share/licenses/arcana-overlay/LICENSE"
            '';
            meta = {
              description = "Native click-through desktop text overlay";
              homepage = "https://github.com/TarocchiLive/arcana-world";
              license = pkgs.lib.licenses.gpl3Only;
              mainProgram = "arcana-overlay";
              platforms = systems;
            };
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
          arcana-world-desktop = pkgs.runCommand "arcana-world-desktop-${arcana-world.version}" {
            meta = arcana-world.meta // {
              description = "Arcana World with optional desktop overlay and speech helper";
            };
          } ''
            install -Dm755 ${arcana-world}/bin/arcana-world "$out/bin/arcana-world"
            install -Dm755 ${arcana-overlay}/bin/arcana-overlay "$out/bin/libexec/arcana-overlay"
            install -Dm755 ${arcana-world-tts}/bin/arcana-world-tts "$out/bin/libexec/arcana-world-tts"
            install -Dm644 ${arcana-world}/share/licenses/arcana-world/LICENSE \
              "$out/share/licenses/arcana-world/LICENSE"
          '';
        }
      );

      devShells = forAllSystems (
        system:
        let
          pkgs = import nixpkgs { inherit system; };
        in
        rec {
          default = arcana-world;
          arcana-world = pkgs.mkShell {
            packages = [ pkgs.go ];
            CGO_ENABLED = "0";
            GOFLAGS = "-mod=readonly";
          };
          arcana-overlay = pkgs.mkShell {
            packages = [ pkgs.go ] ++ pkgs.lib.optionals pkgs.stdenv.isLinux [ pkgs.pkg-config ];
            buildInputs = pkgs.lib.optionals pkgs.stdenv.isLinux [
              pkgs.wayland
              pkgs.pango
              pkgs.cairo
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
        arcana-overlay = {
          type = "app";
          program = "${self.packages.${system}.arcana-overlay}/bin/arcana-overlay";
          meta.description = self.packages.${system}.arcana-overlay.meta.description;
        };
        arcana-world-desktop = {
          type = "app";
          program = "${self.packages.${system}.arcana-world-desktop}/bin/arcana-world";
          meta.description = self.packages.${system}.arcana-world-desktop.meta.description;
        };
      });

      checks = forAllSystems (system: {
        inherit (self.packages.${system}) arcana-world arcana-overlay arcana-world-tts arcana-world-desktop;
      });
    };
}
