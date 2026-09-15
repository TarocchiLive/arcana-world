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
          vendorHash = "sha256-Rmwrc2K3C0lqF4f27y7Mwq3Am7w5n7opi+bdWPZihKA=";
        in
        rec {
          default = arcana-world;
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
      });

      checks = forAllSystems (system: {
        inherit (self.packages.${system}) arcana-world arcana-overlay;
      });
    };
}
