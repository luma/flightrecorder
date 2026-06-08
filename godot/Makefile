GODOT ?= $(HOME)/godot/Godot_v4.6.2-stable_linux.x86_64
GUT_FLAGS ?= -gconfig=res://.gutconfig.json
TEST_ARTIFACT_DIR ?= $(CURDIR)/.godot-test
GODOT_FLAGS ?= --headless --path "$(CURDIR)" --log-file "$(TEST_ARTIFACT_DIR)/godot.log"

# Godot 4.6.2/GUT 9.6 selected runs crash when --log-file is combined with -gselect.
.PHONY: test
test:
	mkdir -p "$(TEST_ARTIFACT_DIR)"
	$(GODOT) $(GODOT_FLAGS) -s addons/gut/gut_cmdln.gd $(GUT_FLAGS)

.PHONY: test-file
test-file:
	$(if $(FILE),,$(error FILE is required, e.g. FILE=res://tests/test_flightrecorder_telemetry.gd))
	$(GODOT) --headless --path "$(CURDIR)" -s addons/gut/gut_cmdln.gd $(GUT_FLAGS) -gselect=$(notdir $(FILE))
