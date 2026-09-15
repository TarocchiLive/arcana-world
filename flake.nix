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

            vendorHash = "sha256-WU7NRrYE8n8zjAL9TRBUsUfCknHDJdWMVHRN9hW/hkI=";
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
        }
      );

      apps = forAllSystems (system: {
        default = {
          type = "app";
          program = "${self.packages.${system}.default}/bin/arcana-world";
          meta.description = self.packages.${system}.default.meta.description;
        };
      });

      checks = forAllSystems (system: {
        inherit (self.packages.${system}) arcana-world;
      });
    };
}
