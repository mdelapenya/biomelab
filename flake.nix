{
  description = "biomelab — a dashboard for git worktrees and coding agents sandboxes";

  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";
    flake-utils.url = "github:numtide/flake-utils";
  };

  outputs = { self, nixpkgs, flake-utils }:
    flake-utils.lib.eachDefaultSystem (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in
      {
        packages.default = pkgs.buildGoModule {
          pname = "biomelab";
          version = "unstable-${self.shortRev or self.dirtyShortRev or "unknown"}";

          src = self;

          # To update: run `nix build` and replace with the hash from the error message.
          vendorHash = "sha256-AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=";

          subPackages = [ "cmd/biomelab" ];

          meta = with pkgs.lib; {
            description = "BiomeLab — a dashboard for git worktrees and coding agents sandboxes";
            homepage = "https://github.com/mdelapenya/biomelab";
            license = licenses.mit;
            mainProgram = "biomelab";
          };
        };

        devShells.default = pkgs.mkShell {
          buildInputs = with pkgs; [
            go
            go-task
            golangci-lint
          ];
        };
      });
}
