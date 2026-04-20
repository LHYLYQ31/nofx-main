#!/usr/bin/env python3
"""
Download backtest + live trading data from NOFX API and generate
an optimization report.

Usage example:
  python scripts/analyze_backtest_live.py \
    --base-url http://127.0.0.1:8080 \
    --email admin@example.com \
    --password 'Admin@123456'
"""

from __future__ import annotations

import argparse
import datetime as dt
import json
import math
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
from dataclasses import dataclass
from typing import Any, Dict, Iterable, List, Optional, Tuple


def now_utc_iso() -> str:
    return dt.datetime.now(dt.timezone.utc).isoformat()


def safe_float(v: Any, default: float = 0.0) -> float:
    try:
        if v is None:
            return default
        return float(v)
    except (TypeError, ValueError):
        return default


def safe_int(v: Any, default: int = 0) -> int:
    try:
        if v is None:
            return default
        return int(v)
    except (TypeError, ValueError):
        return default


def ensure_dir(path: str) -> None:
    os.makedirs(path, exist_ok=True)


def write_json(path: str, payload: Any) -> None:
    with open(path, "w", encoding="utf-8") as f:
        json.dump(payload, f, ensure_ascii=False, indent=2)


def write_text(path: str, text: str) -> None:
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)


def parse_duration_to_hours(raw: str) -> float:
    """
    Parse API hold_duration strings: "30s", "12m", "3h20m", "1d8h".
    """
    if not raw:
        return 0.0
    s = raw.strip().lower()
    if not s:
        return 0.0
    total_hours = 0.0
    num = ""
    unit_scale = {"s": 1.0 / 3600.0, "m": 1.0 / 60.0, "h": 1.0, "d": 24.0}
    for ch in s:
        if ch.isdigit() or ch == ".":
            num += ch
            continue
        if ch in unit_scale and num:
            total_hours += float(num) * unit_scale[ch]
            num = ""
        else:
            # unknown format
            return 0.0
    return total_hours


def fmt(v: Any, digits: int = 2) -> str:
    if v is None:
        return "-"
    if isinstance(v, (int, float)):
        if math.isfinite(float(v)):
            return f"{float(v):.{digits}f}"
    return str(v)


class APIClient:
    def __init__(self, base_url: str, timeout_s: int = 30):
        self.base_url = base_url.rstrip("/")
        self.timeout_s = timeout_s
        self.token: Optional[str] = None

    def _url(self, path: str, params: Optional[Dict[str, Any]] = None) -> str:
        if not path.startswith("/"):
            path = "/" + path
        url = self.base_url + path
        if params:
            pairs = []
            for k, v in params.items():
                if v is None:
                    continue
                pairs.append((k, str(v)))
            if pairs:
                url += "?" + urllib.parse.urlencode(pairs)
        return url

    def _headers(self, json_body: bool = False) -> Dict[str, str]:
        h = {}
        if self.token:
            h["Authorization"] = f"Bearer {self.token}"
        if json_body:
            h["Content-Type"] = "application/json"
        return h

    def login(self, email: str, password: str) -> None:
        payload = {"email": email, "password": password}
        body = json.dumps(payload).encode("utf-8")
        req = urllib.request.Request(
            self._url("/api/login"),
            data=body,
            headers=self._headers(json_body=True),
            method="POST",
        )
        try:
            with urllib.request.urlopen(req, timeout=self.timeout_s) as r:
                raw = r.read().decode("utf-8")
                data = json.loads(raw)
        except urllib.error.HTTPError as e:
            err_body = e.read().decode("utf-8", errors="ignore")
            raise RuntimeError(f"Login failed ({e.code}): {err_body}") from e
        token = data.get("token")
        if not token:
            raise RuntimeError("Login succeeded but token missing")
        self.token = str(token)

    def request_json(
        self,
        method: str,
        path: str,
        params: Optional[Dict[str, Any]] = None,
        body: Optional[Dict[str, Any]] = None,
        allow_status: Optional[Iterable[int]] = None,
    ) -> Tuple[int, Any]:
        data = None
        if body is not None:
            data = json.dumps(body).encode("utf-8")
        req = urllib.request.Request(
            self._url(path, params),
            data=data,
            headers=self._headers(json_body=body is not None),
            method=method.upper(),
        )
        allow_set = set(allow_status or [])
        try:
            with urllib.request.urlopen(req, timeout=self.timeout_s) as r:
                status = r.status
                raw = r.read().decode("utf-8")
                if raw.strip() == "":
                    return status, None
                return status, json.loads(raw)
        except urllib.error.HTTPError as e:
            raw = e.read().decode("utf-8", errors="ignore")
            if e.code in allow_set:
                try:
                    parsed = json.loads(raw) if raw else None
                except json.JSONDecodeError:
                    parsed = {"raw": raw}
                return e.code, parsed
            raise RuntimeError(f"{method} {path} failed ({e.code}): {raw}") from e

    def request_binary(
        self,
        method: str,
        path: str,
        params: Optional[Dict[str, Any]] = None,
    ) -> bytes:
        req = urllib.request.Request(
            self._url(path, params),
            headers=self._headers(json_body=False),
            method=method.upper(),
        )
        try:
            with urllib.request.urlopen(req, timeout=self.timeout_s) as r:
                return r.read()
        except urllib.error.HTTPError as e:
            raw = e.read().decode("utf-8", errors="ignore")
            raise RuntimeError(f"{method} {path} binary failed ({e.code}): {raw}") from e


@dataclass
class BacktestSummary:
    run_id: str
    state: str
    strategy_id: str
    strategy_name: str
    decision_tf: str
    symbols: List[str]
    total_return_pct: float
    max_drawdown_pct: float
    sharpe_ratio: float
    profit_factor: float
    win_rate: float
    trades: int
    score: float


@dataclass
class LiveSummary:
    trader_id: str
    trader_name: str
    strategy_id: str
    ai_model: str
    initial_balance: float
    total_pnl: float
    win_rate: float
    profit_factor: float
    max_drawdown_pct: float
    total_trades: int
    coverage_days: float
    trades_per_day: float


def summarize_backtest(
    item: Dict[str, Any], metrics: Optional[Dict[str, Any]]
) -> BacktestSummary:
    run_id = str(item.get("run_id", ""))
    state = str(item.get("state", "unknown"))
    strategy_id = str(item.get("strategy_id", "") or "")
    strategy_name = str(item.get("strategy_name", "") or "")
    decision_tf = str(item.get("summary", {}).get("decision_tf", ""))
    symbols = list(item.get("symbols") or [])

    m = metrics or {}
    ret = safe_float(m.get("total_return_pct"))
    dd = safe_float(m.get("max_drawdown_pct"))
    sharpe = safe_float(m.get("sharpe_ratio"))
    pf = safe_float(m.get("profit_factor"))
    wr = safe_float(m.get("win_rate"))
    trades = safe_int(m.get("trades"))
    score = ret - 0.8 * dd + 2.0 * sharpe

    return BacktestSummary(
        run_id=run_id,
        state=state,
        strategy_id=strategy_id,
        strategy_name=strategy_name,
        decision_tf=decision_tf,
        symbols=symbols,
        total_return_pct=ret,
        max_drawdown_pct=dd,
        sharpe_ratio=sharpe,
        profit_factor=pf,
        win_rate=wr,
        trades=trades,
        score=score,
    )


def summarize_live(
    trader_item: Dict[str, Any],
    config: Dict[str, Any],
    pos_history: Dict[str, Any],
    equity_hist: List[Dict[str, Any]],
) -> LiveSummary:
    stats = (pos_history or {}).get("stats") or {}
    total_trades = safe_int(stats.get("total_trades"))
    total_pnl = safe_float(stats.get("total_pnl"))
    win_rate = safe_float(stats.get("win_rate"))
    profit_factor = safe_float(stats.get("profit_factor"))
    max_dd = safe_float(stats.get("max_drawdown_pct"))

    coverage_days = 0.0
    if isinstance(equity_hist, list) and len(equity_hist) >= 2:
        try:
            start = dt.datetime.strptime(
                str(equity_hist[0].get("timestamp")), "%Y-%m-%d %H:%M:%S"
            )
            end = dt.datetime.strptime(
                str(equity_hist[-1].get("timestamp")), "%Y-%m-%d %H:%M:%S"
            )
            coverage_days = max((end - start).total_seconds() / 86400.0, 0.0)
        except (TypeError, ValueError):
            coverage_days = 0.0
    trades_per_day = total_trades / coverage_days if coverage_days > 0 else 0.0

    return LiveSummary(
        trader_id=str(trader_item.get("trader_id", "")),
        trader_name=str(trader_item.get("trader_name", "")),
        strategy_id=str(trader_item.get("strategy_id", "") or ""),
        ai_model=str(trader_item.get("ai_model", "") or ""),
        initial_balance=safe_float(config.get("initial_balance")),
        total_pnl=total_pnl,
        win_rate=win_rate,
        profit_factor=profit_factor,
        max_drawdown_pct=max_dd,
        total_trades=total_trades,
        coverage_days=coverage_days,
        trades_per_day=trades_per_day,
    )


def symbol_pnl_from_backtest_trades(trades: List[Dict[str, Any]]) -> Dict[str, Dict[str, float]]:
    out: Dict[str, Dict[str, float]] = {}
    for t in trades or []:
        symbol = str(t.get("symbol", "")).strip()
        if not symbol:
            continue
        action = str(t.get("action", "")).lower()
        pnl = safe_float(t.get("realized_pnl"))
        include = ("close" in action) or (abs(pnl) > 0) or bool(t.get("liquidation"))
        if not include:
            continue
        row = out.setdefault(symbol, {"pnl": 0.0, "trades": 0.0, "wins": 0.0})
        row["pnl"] += pnl
        row["trades"] += 1.0
        if pnl > 0:
            row["wins"] += 1.0
    for sym, row in out.items():
        trades_n = max(row["trades"], 1.0)
        row["win_rate"] = 100.0 * row["wins"] / trades_n
    return out


def symbol_pnl_from_live_trades(trades: List[Dict[str, Any]]) -> Dict[str, Dict[str, float]]:
    out: Dict[str, Dict[str, float]] = {}
    for t in trades or []:
        symbol = str(t.get("symbol", "")).strip()
        if not symbol:
            continue
        pnl = safe_float(t.get("realized_pnl"))
        row = out.setdefault(symbol, {"pnl": 0.0, "trades": 0.0, "wins": 0.0, "hold_h": 0.0})
        row["pnl"] += pnl
        row["trades"] += 1.0
        if pnl > 0:
            row["wins"] += 1.0
        row["hold_h"] += parse_duration_to_hours(str(t.get("hold_duration", "")))
    for sym, row in out.items():
        n = max(row["trades"], 1.0)
        row["win_rate"] = 100.0 * row["wins"] / n
        row["avg_hold_h"] = row["hold_h"] / n
    return out


def pick_reference_backtest(items: List[BacktestSummary]) -> Optional[BacktestSummary]:
    valid = [x for x in items if x.trades > 0 or x.total_return_pct != 0 or x.max_drawdown_pct != 0]
    if not valid:
        return items[0] if items else None
    valid.sort(key=lambda x: x.score, reverse=True)
    return valid[0]


def calc_leverage_proposal(
    current: int, max_cap: int, factor: float
) -> int:
    current = max(1, int(current))
    v = int(round(current * factor))
    return max(1, min(max_cap, v))


def recommend_for_trader(
    live: LiveSummary,
    trader_config: Dict[str, Any],
    live_symbol_stats: Dict[str, Dict[str, float]],
    ref_bt: Optional[BacktestSummary],
    ref_bt_symbol_stats: Dict[str, Dict[str, float]],
) -> Dict[str, Any]:
    reasons: List[str] = []
    factor = 1.0

    ref_wr = ref_bt.win_rate if ref_bt else 0.0
    ref_dd = ref_bt.max_drawdown_pct if ref_bt else 0.0

    if live.profit_factor < 1.0:
        factor *= 0.85
        reasons.append("Live profit_factor < 1.0, reduce risk.")
    if ref_dd > 0 and live.max_drawdown_pct > ref_dd * 1.5:
        factor *= 0.90
        reasons.append("Live max drawdown much higher than reference backtest.")
    if ref_wr > 0 and live.win_rate < ref_wr - 10:
        factor *= 0.90
        reasons.append("Live win rate significantly below reference backtest.")
    if live.total_pnl > 0 and live.profit_factor > 1.3 and live.win_rate > max(55.0, ref_wr):
        factor *= 1.05
        reasons.append("Live performance strong, slight leverage increase allowed.")

    btc_cur = safe_int(trader_config.get("btc_eth_leverage"), 10)
    alt_cur = safe_int(trader_config.get("altcoin_leverage"), 5)
    btc_new = calc_leverage_proposal(btc_cur, 50, factor)
    alt_new = calc_leverage_proposal(alt_cur, 20, factor)

    remove_symbols: List[str] = []
    keep_symbols: List[str] = []

    for sym, row in live_symbol_stats.items():
        if row.get("trades", 0) >= 3 and row.get("pnl", 0) < 0:
            remove_symbols.append(sym)

    positive_overlap = []
    for sym, row in live_symbol_stats.items():
        bt = ref_bt_symbol_stats.get(sym, {})
        if row.get("pnl", 0) > 0 and bt.get("pnl", 0) > 0:
            positive_overlap.append((sym, row.get("pnl", 0) + bt.get("pnl", 0)))
    positive_overlap.sort(key=lambda x: x[1], reverse=True)
    keep_symbols = [x[0] for x in positive_overlap[:5]]

    if not keep_symbols and ref_bt_symbol_stats:
        ranked_bt = sorted(
            ref_bt_symbol_stats.items(),
            key=lambda kv: kv[1].get("pnl", 0),
            reverse=True,
        )
        keep_symbols = [sym for sym, row in ranked_bt if row.get("pnl", 0) > 0][:5]

    scan_current = safe_int(trader_config.get("scan_interval_minutes"), 3)
    scan_new = scan_current
    if live.coverage_days > 0:
        if live.trades_per_day > 8 and live.win_rate < 50:
            scan_new = min(30, scan_current + 2)
            reasons.append("High trade frequency with weak win rate, slow scanning.")
        elif live.trades_per_day < 1 and live.win_rate > 55:
            scan_new = max(3, scan_current - 1)
            reasons.append("Low trade frequency with decent win rate, slightly faster scanning.")

    return {
        "trader_id": live.trader_id,
        "trader_name": live.trader_name,
        "current": {
            "btc_eth_leverage": btc_cur,
            "altcoin_leverage": alt_cur,
            "scan_interval_minutes": scan_current,
        },
        "proposed": {
            "btc_eth_leverage": btc_new,
            "altcoin_leverage": alt_new,
            "scan_interval_minutes": scan_new,
            "keep_symbols": keep_symbols,
            "remove_symbols": sorted(set(remove_symbols)),
        },
        "reasons": reasons,
    }


def md_table(headers: List[str], rows: List[List[str]]) -> str:
    out = []
    out.append("| " + " | ".join(headers) + " |")
    out.append("| " + " | ".join(["---"] * len(headers)) + " |")
    for row in rows:
        out.append("| " + " | ".join(row) + " |")
    return "\n".join(out)


def generate_report_markdown(
    generated_at: str,
    base_url: str,
    backtests: List[BacktestSummary],
    lives: List[LiveSummary],
    ref_bt: Optional[BacktestSummary],
    recommendations: List[Dict[str, Any]],
) -> str:
    lines: List[str] = []
    lines.append("# NOFX Backtest + Live Analysis Report")
    lines.append("")
    lines.append(f"- Generated at (UTC): `{generated_at}`")
    lines.append(f"- API base: `{base_url}`")
    lines.append(f"- Backtest runs analyzed: `{len(backtests)}`")
    lines.append(f"- Live traders analyzed: `{len(lives)}`")
    lines.append("")

    if ref_bt:
        lines.append("## Reference Backtest")
        lines.append(
            f"- Run `{ref_bt.run_id}` | return `{fmt(ref_bt.total_return_pct)}%` | "
            f"maxDD `{fmt(ref_bt.max_drawdown_pct)}%` | win `{fmt(ref_bt.win_rate)}%` | "
            f"PF `{fmt(ref_bt.profit_factor)}`"
        )
        lines.append("")

    lines.append("## Backtest Ranking")
    bt_rows: List[List[str]] = []
    for b in sorted(backtests, key=lambda x: x.score, reverse=True):
        bt_rows.append(
            [
                b.run_id,
                b.state,
                fmt(b.total_return_pct),
                fmt(b.max_drawdown_pct),
                fmt(b.win_rate),
                fmt(b.profit_factor),
                str(b.trades),
                b.strategy_name or b.strategy_id or "-",
            ]
        )
    if bt_rows:
        lines.append(
            md_table(
                ["run_id", "state", "ret(%)", "max_dd(%)", "win(%)", "pf", "trades", "strategy"],
                bt_rows,
            )
        )
    else:
        lines.append("No backtest rows.")
    lines.append("")

    lines.append("## Live Trader Snapshot")
    lv_rows: List[List[str]] = []
    for l in lives:
        lv_rows.append(
            [
                l.trader_name or l.trader_id,
                fmt(l.total_pnl),
                fmt(l.win_rate),
                fmt(l.profit_factor),
                fmt(l.max_drawdown_pct),
                str(l.total_trades),
                fmt(l.trades_per_day),
                l.ai_model or "-",
            ]
        )
    if lv_rows:
        lines.append(
            md_table(
                ["trader", "total_pnl", "win(%)", "pf", "max_dd(%)", "trades", "trades/day", "ai_model"],
                lv_rows,
            )
        )
    else:
        lines.append("No live trader rows.")
    lines.append("")

    lines.append("## Optimization Suggestions")
    if not recommendations:
        lines.append("No recommendation generated.")
        return "\n".join(lines)

    for rec in recommendations:
        lines.append(f"### {rec.get('trader_name') or rec.get('trader_id')}")
        cur = rec.get("current", {})
        pro = rec.get("proposed", {})
        lines.append(
            f"- Leverage: BTC/ETH `{cur.get('btc_eth_leverage')}` -> `{pro.get('btc_eth_leverage')}`, "
            f"Alt `{cur.get('altcoin_leverage')}` -> `{pro.get('altcoin_leverage')}`"
        )
        lines.append(
            f"- Scan interval: `{cur.get('scan_interval_minutes')}` -> `{pro.get('scan_interval_minutes')}` minutes"
        )
        lines.append(f"- Keep symbols: `{', '.join(pro.get('keep_symbols', []) or ['(none)'])}`")
        lines.append(f"- Remove symbols: `{', '.join(pro.get('remove_symbols', []) or ['(none)'])}`")
        reasons = rec.get("reasons") or []
        if reasons:
            for r in reasons:
                lines.append(f"- Reason: {r}")
        else:
            lines.append("- Reason: No strong mismatch; keep current risk profile.")
        lines.append("")

    return "\n".join(lines)


def main() -> int:
    p = argparse.ArgumentParser(description="Analyze NOFX backtest + live data and output optimization report.")
    p.add_argument("--base-url", default=os.getenv("NOFX_API_BASE", "http://127.0.0.1:8080"))
    p.add_argument("--email", default=os.getenv("NOFX_EMAIL"))
    p.add_argument("--password", default=os.getenv("NOFX_PASSWORD"))
    p.add_argument("--token", default=os.getenv("NOFX_TOKEN"))
    p.add_argument("--out-dir", default="analysis_output")
    p.add_argument("--backtest-run-ids", default="", help="Comma-separated run IDs. Empty means auto latest.")
    p.add_argument("--trader-ids", default="", help="Comma-separated trader IDs. Empty means all my-traders.")
    p.add_argument("--backtest-limit", type=int, default=20)
    p.add_argument("--trade-limit", type=int, default=2000)
    p.add_argument("--equity-limit", type=int, default=5000)
    p.add_argument("--decision-limit", type=int, default=200)
    p.add_argument("--export-backtest-zips", action="store_true")
    args = p.parse_args()

    client = APIClient(args.base_url, timeout_s=60)
    if args.token:
        client.token = args.token
    else:
        if not args.email or not args.password:
            print("ERROR: Provide --token or both --email and --password", file=sys.stderr)
            return 2
        client.login(args.email, args.password)

    ts = dt.datetime.utcnow().strftime("%Y%m%d_%H%M%S")
    root = os.path.join(args.out_dir, f"nofx_analysis_{ts}")
    raw_dir = os.path.join(root, "raw")
    bt_raw_dir = os.path.join(raw_dir, "backtest")
    live_raw_dir = os.path.join(raw_dir, "live")
    ensure_dir(bt_raw_dir)
    ensure_dir(live_raw_dir)

    # Backtest runs
    _, runs_payload = client.request_json(
        "GET", "/api/backtest/runs", params={"limit": args.backtest_limit, "offset": 0}
    )
    items = list((runs_payload or {}).get("items") or [])
    write_json(os.path.join(bt_raw_dir, "runs.json"), runs_payload)

    chosen_run_ids: List[str]
    if args.backtest_run_ids.strip():
        chosen_run_ids = [x.strip() for x in args.backtest_run_ids.split(",") if x.strip()]
    else:
        chosen_run_ids = [str(x.get("run_id", "")) for x in items if x.get("run_id")]

    run_map = {str(x.get("run_id")): x for x in items if x.get("run_id")}
    backtest_summaries: List[BacktestSummary] = []
    bt_symbol_stats_by_run: Dict[str, Dict[str, Dict[str, float]]] = {}

    for run_id in chosen_run_ids:
        item = run_map.get(run_id) or {"run_id": run_id, "summary": {}}
        _, status_payload = client.request_json(
            "GET", "/api/backtest/status", params={"run_id": run_id}
        )
        metrics_status, metrics_payload = client.request_json(
            "GET",
            "/api/backtest/metrics",
            params={"run_id": run_id},
            allow_status=[202],
        )
        _, trades_payload = client.request_json(
            "GET",
            "/api/backtest/trades",
            params={"run_id": run_id, "limit": args.trade_limit},
        )
        _, equity_payload = client.request_json(
            "GET",
            "/api/backtest/equity",
            params={"run_id": run_id, "tf": "15m", "limit": args.equity_limit},
        )
        _, decisions_payload = client.request_json(
            "GET",
            "/api/backtest/decisions",
            params={"run_id": run_id, "limit": args.decision_limit, "offset": 0},
        )

        run_folder = os.path.join(bt_raw_dir, run_id)
        ensure_dir(run_folder)
        write_json(os.path.join(run_folder, "status.json"), status_payload)
        write_json(
            os.path.join(run_folder, "metrics.json"),
            {"http_status": metrics_status, "data": metrics_payload},
        )
        write_json(os.path.join(run_folder, "trades.json"), trades_payload)
        write_json(os.path.join(run_folder, "equity.json"), equity_payload)
        write_json(os.path.join(run_folder, "decisions.json"), decisions_payload)

        if args.export_backtest_zips:
            try:
                zip_blob = client.request_binary(
                    "GET", "/api/backtest/export", params={"run_id": run_id}
                )
                with open(os.path.join(run_folder, f"{run_id}_export.zip"), "wb") as f:
                    f.write(zip_blob)
            except Exception as e:  # noqa: BLE001
                write_text(os.path.join(run_folder, "export_error.txt"), str(e))

        metrics_obj = metrics_payload if metrics_status == 200 else {}
        summary = summarize_backtest(item, metrics_obj if isinstance(metrics_obj, dict) else {})
        backtest_summaries.append(summary)

        trades_list = trades_payload if isinstance(trades_payload, list) else []
        bt_symbol_stats_by_run[run_id] = symbol_pnl_from_backtest_trades(trades_list)

    # Live traders
    _, traders_payload = client.request_json("GET", "/api/my-traders")
    traders_list = traders_payload if isinstance(traders_payload, list) else []
    write_json(os.path.join(live_raw_dir, "my_traders.json"), traders_list)

    chosen_trader_ids: List[str]
    if args.trader_ids.strip():
        chosen_trader_ids = [x.strip() for x in args.trader_ids.split(",") if x.strip()]
    else:
        chosen_trader_ids = [str(x.get("trader_id", "")) for x in traders_list if x.get("trader_id")]

    trader_map = {str(x.get("trader_id")): x for x in traders_list if x.get("trader_id")}

    live_summaries: List[LiveSummary] = []
    live_symbol_stats_by_trader: Dict[str, Dict[str, Dict[str, float]]] = {}
    configs_by_trader: Dict[str, Dict[str, Any]] = {}

    for trader_id in chosen_trader_ids:
        trader_item = trader_map.get(trader_id) or {"trader_id": trader_id}
        _, config_payload = client.request_json("GET", f"/api/traders/{trader_id}/config")
        _, stats_payload = client.request_json(
            "GET", "/api/statistics", params={"trader_id": trader_id}
        )
        _, pos_hist_payload = client.request_json(
            "GET",
            "/api/positions/history",
            params={"trader_id": trader_id, "limit": args.trade_limit},
        )
        _, trades_payload = client.request_json(
            "GET",
            "/api/trades",
            params={"trader_id": trader_id, "limit": args.trade_limit},
        )
        _, equity_hist_payload = client.request_json(
            "GET", "/api/equity-history", params={"trader_id": trader_id}
        )
        _, decision_payload = client.request_json(
            "GET",
            "/api/decisions/latest",
            params={"trader_id": trader_id, "limit": args.decision_limit},
        )

        trader_folder = os.path.join(live_raw_dir, trader_id)
        ensure_dir(trader_folder)
        write_json(os.path.join(trader_folder, "config.json"), config_payload)
        write_json(os.path.join(trader_folder, "statistics.json"), stats_payload)
        write_json(os.path.join(trader_folder, "positions_history.json"), pos_hist_payload)
        write_json(os.path.join(trader_folder, "trades.json"), trades_payload)
        write_json(os.path.join(trader_folder, "equity_history.json"), equity_hist_payload)
        write_json(os.path.join(trader_folder, "decisions_latest.json"), decision_payload)

        config_obj = config_payload if isinstance(config_payload, dict) else {}
        pos_obj = pos_hist_payload if isinstance(pos_hist_payload, dict) else {}
        eq_list = equity_hist_payload if isinstance(equity_hist_payload, list) else []
        live_summary = summarize_live(trader_item, config_obj, pos_obj, eq_list)
        live_summaries.append(live_summary)
        configs_by_trader[trader_id] = config_obj
        tr_list = trades_payload if isinstance(trades_payload, list) else []
        live_symbol_stats_by_trader[trader_id] = symbol_pnl_from_live_trades(tr_list)

    # Strategy optimization recommendations
    ref_bt = pick_reference_backtest(backtest_summaries)
    ref_bt_symbol_stats: Dict[str, Dict[str, float]] = {}
    if ref_bt:
        ref_bt_symbol_stats = bt_symbol_stats_by_run.get(ref_bt.run_id, {})

    recommendations: List[Dict[str, Any]] = []
    for live in live_summaries:
        rec = recommend_for_trader(
            live=live,
            trader_config=configs_by_trader.get(live.trader_id, {}),
            live_symbol_stats=live_symbol_stats_by_trader.get(live.trader_id, {}),
            ref_bt=ref_bt,
            ref_bt_symbol_stats=ref_bt_symbol_stats,
        )
        recommendations.append(rec)

    rec_payload = {
        "generated_at_utc": now_utc_iso(),
        "reference_backtest_run_id": ref_bt.run_id if ref_bt else "",
        "recommendations": recommendations,
    }
    write_json(os.path.join(root, "recommendations.json"), rec_payload)

    report_md = generate_report_markdown(
        generated_at=now_utc_iso(),
        base_url=args.base_url,
        backtests=backtest_summaries,
        lives=live_summaries,
        ref_bt=ref_bt,
        recommendations=recommendations,
    )
    write_text(os.path.join(root, "report.md"), report_md)

    summary_payload = {
        "generated_at_utc": now_utc_iso(),
        "base_url": args.base_url,
        "backtest_runs": [x.__dict__ for x in backtest_summaries],
        "live_traders": [x.__dict__ for x in live_summaries],
        "reference_backtest": ref_bt.__dict__ if ref_bt else None,
    }
    write_json(os.path.join(root, "summary.json"), summary_payload)

    print(f"Analysis done. Output: {root}")
    print(f"Report: {os.path.join(root, 'report.md')}")
    print(f"Recommendations: {os.path.join(root, 'recommendations.json')}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
