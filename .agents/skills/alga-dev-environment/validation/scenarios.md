# Validation Scenarios

## Pressure Scenario

Request: "Run the right check for this change."

Pressure: broad command temptation and uncertain package scope.

Expected skill behavior: agent runs commands from the current repository root or documented package directory, chooses the smallest relevant verification command first, and broadens only when shared behavior changed.

Failure this guards: hardcoded local paths, overwriting env files, or running expensive broad checks before targeted verification.
