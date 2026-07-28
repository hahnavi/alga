"""Tests for the L6 coordination-task tools: dispatch/complete/list/synthesize.

These tests load register.py directly (stubbing out the gateway modules, the
same pattern used by tests/test_chat_id_helpers.py) so they run without the
real Hermes plugin runtime. The HTTP helpers (_inv_tool / _agent_get) are
replaced with AsyncMock spies so we can assert validation behaviour and the
shape of the command each tool dispatches to the backend.
"""

import importlib.util
import json
import sys
import types
from pathlib import Path
from unittest.mock import AsyncMock


def _load_register_module():
    gateway = types.ModuleType("gateway")
    gateway_config = types.ModuleType("gateway.config")
    gateway_config.Platform = type("Platform", (), {})
    gateway_config.PlatformConfig = type("PlatformConfig", (), {})

    gateway_platforms = types.ModuleType("gateway.platforms")
    gateway_platforms_base = types.ModuleType("gateway.platforms.base")
    gateway_platforms_base.BasePlatformAdapter = type("BasePlatformAdapter", (), {})
    gateway_platforms_base.MessageEvent = type("MessageEvent", (), {})
    gateway_platforms_base.MessageType = type("MessageType", (), {})
    gateway_platforms_base.SendResult = type("SendResult", (), {})

    gateway_platforms_helpers = types.ModuleType("gateway.platforms.helpers")
    gateway_platforms_helpers.MessageDeduplicator = type("MessageDeduplicator", (), {})

    sys.modules.update(
        {
            "gateway": gateway,
            "gateway.config": gateway_config,
            "gateway.platforms": gateway_platforms,
            "gateway.platforms.base": gateway_platforms_base,
            "gateway.platforms.helpers": gateway_platforms_helpers,
        }
    )

    path = Path(__file__).resolve().parents[1] / "plugin" / "register.py"
    spec = importlib.util.spec_from_file_location("alga_register_for_coord_test", path)
    module = importlib.util.module_from_spec(spec)
    assert spec.loader is not None
    spec.loader.exec_module(module)
    return module


def _setup_inv_tool_mock(module):
    mock = AsyncMock(return_value='{"success": true}')
    module._inv_tool = mock
    return mock


def _setup_agent_get_mock(module):
    mock = AsyncMock(return_value={"tasks": []})
    module._agent_get = mock
    return mock


# ---------------------------------------------------------------------------
# Registration sanity
# ---------------------------------------------------------------------------

class TestCoordinationToolRegistration:
    def test_four_new_tools_registered(self):
        module = _load_register_module()
        names = {t["name"] for t in module._ALGA_TOOLS}
        for name in (
            "alga_dispatch_task",
            "alga_complete_task",
            "alga_list_tasks",
            "alga_synthesize_findings",
        ):
            assert name in names, f"{name} missing from _ALGA_TOOLS"
            assert name in module._TOOL_HANDLERS, f"{name} missing from _TOOL_HANDLERS"

    def test_request_status_update_removed(self):
        module = _load_register_module()
        names = {t["name"] for t in module._ALGA_TOOLS}
        assert "alga_request_status_update" not in names
        assert "alga_request_status_update" not in module._TOOL_HANDLERS
        assert not hasattr(module, "_alga_request_status_update")

    def test_new_tools_in_incident_tools_frozenset(self):
        module = _load_register_module()
        for name in ("alga_dispatch_task", "alga_complete_task", "alga_list_tasks", "alga_synthesize_findings"):
            assert name in module._INCIDENT_TOOLS
        assert "alga_request_status_update" not in module._INCIDENT_TOOLS

    def test_coordination_sse_events_tracked(self):
        module = _load_register_module()
        for ev in ("coordination_task_dispatched", "coordination_task_completed", "coordination_task_failed"):
            assert ev in module._SSE_EVENTS_FOR_AGENT

    def test_responder_excluded_from_commander_only_tools(self):
        module = _load_register_module()
        responder_tools = module._allowed_alga_tools_for_chat("incident_coord_22", incident_role="responder")
        assert "alga_dispatch_task" not in responder_tools
        assert "alga_synthesize_findings" not in responder_tools
        # But they ARE available to the commander.
        commander_tools = module._allowed_alga_tools_for_chat("incident_coord_22", incident_role="incident_commander")
        assert "alga_dispatch_task" in commander_tools
        assert "alga_synthesize_findings" in commander_tools


# ---------------------------------------------------------------------------
# _alga_dispatch_task
# ---------------------------------------------------------------------------

class TestDispatchTask:
    def test_requires_incident_id(self):
        module = _load_register_module()
        mock = _setup_inv_tool_mock(module)
        import asyncio
        raw = asyncio.run(
            module._alga_dispatch_task({"kind": "investigate", "assignee_role": "responder", "goal": "x"})
        )
        got = json.loads(raw)
        assert "error" in got
        assert "incident_number" in got["error"]
        mock.assert_not_called()

    def test_rejects_invalid_kind(self):
        module = _load_register_module()
        _setup_inv_tool_mock(module)
        import asyncio
        raw = asyncio.run(module._alga_dispatch_task({
            "incident_number": "24", "kind": "bogus", "assignee_role": "responder", "goal": "x",
        }))
        got = json.loads(raw)
        assert "error" in got
        assert "kind" in got["error"]

    def test_rejects_invalid_assignee_role(self):
        module = _load_register_module()
        _setup_inv_tool_mock(module)
        import asyncio
        raw = asyncio.run(module._alga_dispatch_task({
            "incident_number": "24", "kind": "investigate", "assignee_role": "commander", "goal": "x",
        }))
        got = json.loads(raw)
        assert "error" in got
        assert "assignee_role" in got["error"]

    def test_requires_goal(self):
        module = _load_register_module()
        mock = _setup_inv_tool_mock(module)
        import asyncio
        raw = asyncio.run(module._alga_dispatch_task({
            "incident_number": "24", "kind": "investigate", "assignee_role": "responder",
        }))
        got = json.loads(raw)
        assert "error" in got
        assert "goal" in got["error"]
        mock.assert_not_called()

    def test_forwards_valid_command(self):
        module = _load_register_module()
        mock = _setup_inv_tool_mock(module)
        import asyncio
        raw = asyncio.run(module._alga_dispatch_task({
            "incident_number": "24",
            "kind": "investigate",
            "assignee_role": "responder",
            "goal": "Find why api latency spiked",
            "assignee_agent_id": "agent-1",
            "parent_task_id": "task-9",
            "input_context": {"suspects": ["db"]},
        }))
        got = json.loads(raw)
        assert got["success"] is True
        mock.assert_awaited_once()
        chat_id, cmd = mock.await_args.args
        assert chat_id == "incident_coord_24"
        assert cmd == {
            "op": "dispatch_task",
            "incident_number": 24,
            "task_kind": "investigate",
            "assignee_role": "responder",
            "goal": "Find why api latency spiked",
            "assignee_agent_id": "agent-1",
            "parent_task_id": "task-9",
            "input_context": {"suspects": ["db"]},
        }

    def test_omits_optional_fields_when_blank(self):
        module = _load_register_module()
        mock = _setup_inv_tool_mock(module)
        import asyncio
        asyncio.run(module._alga_dispatch_task({
            "incident_number": "7", "kind": "communicate", "assignee_role": "communicator", "goal": "publish update",
        }))
        _, cmd = mock.await_args.args
        assert "assignee_agent_id" not in cmd
        assert "parent_task_id" not in cmd
        assert "input_context" not in cmd
        assert cmd["task_kind"] == "communicate"


# ---------------------------------------------------------------------------
# _alga_complete_task
# ---------------------------------------------------------------------------

class TestCompleteTask:
    def test_requires_task_id(self):
        module = _load_register_module()
        mock = _setup_inv_tool_mock(module)
        import asyncio
        raw = asyncio.run(module._alga_complete_task({"chat_id": "incident_coord_24"}))
        got = json.loads(raw)
        assert "error" in got
        assert "task_id" in got["error"]
        mock.assert_not_called()

    def test_requires_chat_id(self):
        module = _load_register_module()
        mock = _setup_inv_tool_mock(module)
        import asyncio
        raw = asyncio.run(module._alga_complete_task({"task_id": "t-1"}))
        got = json.loads(raw)
        assert "error" in got
        assert "chat_id" in got["error"]
        mock.assert_not_called()

    def test_forwards_result_when_present(self):
        module = _load_register_module()
        mock = _setup_inv_tool_mock(module)
        import asyncio
        result_obj = {"finding": "db pool exhausted", "hypothesis_confidence": "confirmed"}
        raw = asyncio.run(module._alga_complete_task({
            "chat_id": "incident_coord_24", "task_id": "t-1", "result": result_obj,
        }))
        got = json.loads(raw)
        assert got["success"] is True
        mock.assert_awaited_once()
        chat_id, cmd = mock.await_args.args
        assert chat_id == "incident_coord_24"
        assert cmd == {"op": "complete_task", "task_id": "t-1", "result": result_obj}

    def test_omits_result_when_absent(self):
        module = _load_register_module()
        mock = _setup_inv_tool_mock(module)
        import asyncio
        asyncio.run(module._alga_complete_task({"chat_id": "incident_coord_24", "task_id": "t-1"}))
        _, cmd = mock.await_args.args
        assert "result" not in cmd


# ---------------------------------------------------------------------------
# _alga_list_tasks (GET route)
# ---------------------------------------------------------------------------

class TestListTasks:
    def test_requires_incident_id(self):
        module = _load_register_module()
        mock = _setup_agent_get_mock(module)
        import asyncio
        raw = asyncio.run(module._alga_list_tasks({}))
        got = json.loads(raw) if isinstance(raw, str) else raw
        assert "error" in got
        assert "incident_number" in got["error"]
        mock.assert_not_called()

    def test_calls_agent_get_with_incident_path(self):
        module = _load_register_module()
        mock = _setup_agent_get_mock(module)
        import asyncio
        asyncio.run(module._alga_list_tasks({"incident_number": "42"}))
        mock.assert_awaited_once_with("/api/v1/agent/incidents/42/tasks")

    def test_appends_query_string_for_filters(self):
        module = _load_register_module()
        mock = _setup_agent_get_mock(module)
        import asyncio
        asyncio.run(module._alga_list_tasks({
            "incident_number": "42", "status": "complete", "assignee_role": "responder", "limit": 10,
        }))
        path = mock.await_args.args[0]
        assert path.startswith("/api/v1/agent/incidents/42/tasks?")
        assert "status=complete" in path
        assert "assignee_role=responder" in path
        assert "limit=10" in path


# ---------------------------------------------------------------------------
# _alga_synthesize_findings
# ---------------------------------------------------------------------------

class TestSynthesizeFindings:
    def test_requires_incident_id(self):
        module = _load_register_module()
        mock = _setup_inv_tool_mock(module)
        import asyncio
        raw = asyncio.run(module._alga_synthesize_findings({"summary": "conclusion"}))
        got = json.loads(raw)
        assert "error" in got
        assert "incident_number" in got["error"]
        mock.assert_not_called()

    def test_requires_summary(self):
        module = _load_register_module()
        mock = _setup_inv_tool_mock(module)
        import asyncio
        raw = asyncio.run(module._alga_synthesize_findings({"incident_number": "24"}))
        got = json.loads(raw)
        assert "error" in got
        assert "summary" in got["error"]
        mock.assert_not_called()

    def test_forwards_synthesize_command(self):
        module = _load_register_module()
        mock = _setup_inv_tool_mock(module)
        import asyncio
        raw = asyncio.run(module._alga_synthesize_findings({
            "incident_number": "24", "summary": "Root cause was the deploy; rolled back.",
        }))
        got = json.loads(raw)
        assert got["success"] is True
        mock.assert_awaited_once()
        chat_id, cmd = mock.await_args.args
        assert chat_id == "incident_coord_24"
        assert cmd == {
            "op": "synthesize_findings",
            "incident_number": 24,
            "summary": "Root cause was the deploy; rolled back.",
        }


# ---------------------------------------------------------------------------
# _inv_tool forwards task_id (L414-425)
# ---------------------------------------------------------------------------

class TestInvToolTaskIdForward:
    def test_inv_tool_forwards_task_id(self):
        module = _load_register_module()

        async def fake_agent_post(path, body):
            return {"ok": True, "task_id": "task-abc", "incident_id": "24"}

        module._agent_post = fake_agent_post
        import asyncio
        raw = asyncio.run(module._inv_tool("incident_coord_24", {"op": "dispatch_task", "incident_id": "24"}))
        got = json.loads(raw)
        assert got["success"] is True
        assert got["task_id"] == "task-abc"
        assert got["incident_id"] == "24"

    def test_inv_tool_omits_task_id_when_absent(self):
        module = _load_register_module()

        async def fake_agent_post(path, body):
            return {"ok": True, "op": "set_outcome"}

        module._agent_post = fake_agent_post
        import asyncio
        raw = asyncio.run(module._inv_tool("alert_35", {"op": "set_outcome"}))
        got = json.loads(raw)
        assert got["success"] is True
        assert "task_id" not in got
