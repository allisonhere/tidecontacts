{
  description = "TideContacts - offline-first personal CRM for the terminal";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
  };

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
        "x86_64-darwin"
        "aarch64-darwin"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});

      # TideContacts prints a fixed milestone for --version, so this only
      # names the Nix package.
      version =
        if self ? shortRev then
          "unstable-${self.shortRev}"
        else if self ? dirtyShortRev then
          "unstable-${self.dirtyShortRev}"
        else
          "dev";
    in
    {
      packages = forAllSystems (
        pkgs:
        let
          lib = pkgs.lib;
          # Tools TideContacts shells out to at runtime. Appended (not
          # prepended) to PATH so the user's own choices win when present.
          runtimeTools = lib.optionals pkgs.stdenv.hostPlatform.isLinux [
            pkgs.xdg-utils # xdg-open: open http(s) and mailto references
            pkgs.libnotify # notify-send: reminder notifications (--notify)
          ];
        in
        rec {
          tidecontacts = (pkgs.buildGoModule.override { go = pkgs.go_1_26; }) {
            pname = "tidecontacts";
            inherit version;
            src = lib.cleanSource self;

            # Update after any go.mod/go.sum change: set to lib.fakeHash,
            # build, and copy the "got:" hash from the error.
            vendorHash = "sha256-6/dCesCRjhnNmQaFWj6304zyqBfjjz/R/S3GEt89zn8=";

            subPackages = [
              "cmd/tidecontacts"
              "cmd/tidecontacts-plugin"
            ];
            env.CGO_ENABLED = 0;
            ldflags = [
              "-s"
              "-w"
            ];

            nativeBuildInputs = [ pkgs.makeWrapper ];

            # The smoke tests drive real terminals; keep the Nix build
            # hermetic and fast.
            doCheck = false;

            # TideMail loads a plugin from a directory holding plugin.toml and
            # the executables it names, and the plugin's Advanced CRM action
            # runs the tidecontacts binary bundled beside it. Ship that
            # directory as-is (real files, not symlinks out of it) and expose
            # the app on PATH from there. Link it into TideMail's plugin
            # directory to use it:
            #   ln -s ~/.nix-profile/share/tidemail/plugins/tidecontacts \
            #     ~/.config/tidemail/plugins/tidecontacts
            postInstall = ''
              plugin=$out/share/tidemail/plugins/tidecontacts
              install -Dm644 plugins/tidemail/plugin.toml $plugin/plugin.toml
              mv $out/bin/tidecontacts $out/bin/tidecontacts-plugin $plugin/
            ''
            + lib.optionalString (runtimeTools != [ ]) ''
              wrapProgram $plugin/tidecontacts \
                --suffix PATH : ${lib.makeBinPath runtimeTools}
            ''
            + ''
              ln -s $plugin/tidecontacts $out/bin/tidecontacts
            '';

            meta = {
              description = "Offline-first personal CRM for the terminal, with a TideMail contacts plugin";
              homepage = "https://github.com/allisonhere/tidecontacts";
              mainProgram = "tidecontacts";
              platforms = systems;
            };
          };
          default = tidecontacts;
        }
      );

      apps = forAllSystems (pkgs: {
        default = {
          type = "app";
          program = "${self.packages.${pkgs.stdenv.hostPlatform.system}.tidecontacts}/bin/tidecontacts";
        };
      });

      overlays.default = final: _prev: {
        tidecontacts = self.packages.${final.stdenv.hostPlatform.system}.tidecontacts;
      };

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          inputsFrom = [ self.packages.${pkgs.stdenv.hostPlatform.system}.tidecontacts ];
          packages = [
            pkgs.go_1_26
            pkgs.gopls
            pkgs.golangci-lint
            pkgs.delve
            pkgs.python3 # smoke tests
          ];
          CGO_ENABLED = 0;
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt);
    };
}
