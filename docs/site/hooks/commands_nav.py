import json
import pathlib

from mkdocs.exceptions import PluginError


def on_config(config):
    """Put the generated command navigation under the "Commands" entry of nav."""
    path = pathlib.Path(config.config_file_path).parent / "commands-nav.json"
    if not path.is_file():
        raise PluginError(
            "commands-nav.json is missing: run `go run script/gen-docs.go` first"
        )
    commands = json.loads(path.read_text(encoding="utf-8"))
    for item in config.nav:
        if isinstance(item, dict) and list(item) == ["Commands"]:
            item["Commands"] = commands
            return config
    raise PluginError('nav has no "Commands" entry for the generated command navigation')
