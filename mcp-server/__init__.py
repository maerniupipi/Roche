#!/usr/bin/env python3
"""
RocheKAP MCP Server Package

A Model Context Protocol server that provides access to the RocheKAP knowledge management API.
"""

__version__ = "1.0.0"
__author__ = "RocheKAP Team"
__description__ = "RocheKAP MCP Server - Model Context Protocol server for RocheKAP API"

from .roche_kap_mcp_server import RocheKAPClient, run

__all__ = ["RocheKAPClient", "run"]
