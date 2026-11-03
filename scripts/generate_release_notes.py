#!/usr/bin/env python3
"""
generate_release_notes.py
Antigravity Ecosystem • Gerador Inteligente de Release Notes via IA
Coleta alterações do Git (Mobile e Server) e gera notas dinâmicas com IA.
"""

import argparse
import json
import os
import re
import subprocess
import sys


def get_git_changes(repo_dir: str, repo_name: str) -> list[str]:
    """Coleta commits e mensagens de alteração desde a última release."""
    if not os.path.isdir(repo_dir):
        return []

    try:
        # Busca últimos 20 commits formatados
        res = subprocess.run(
            ["git", "-C", repo_dir, "log", "-n", "20", "--format=%H|%s|%b---END---"],
            capture_output=True,
            text=True,
            check=True,
        )
    except subprocess.SubprocessError:
        return []

    commits = []
    entries = res.stdout.split("---END---")
    found_previous_release = False

    for entry in entries:
        entry = entry.strip()
        if not entry:
            continue
        parts = entry.split("|", 2)
        if len(parts) < 2:
            continue
        _commit_hash, subject = parts[0], parts[1].strip()
        body = parts[2].strip() if len(parts) > 2 else ""

        # Identifica se é commit de release anterior
        is_release_commit = (
            subject.startswith("chore(release):") or
            subject.startswith("chore: release") or
            "bump to v" in subject.lower()
        )

        if is_release_commit:
            if commits:
                # Já coletamos os commits novos dessa iteração
                found_previous_release = True
                break
            else:
                # Se o commit mais recente já for uma release, continua buscando as novidades que a compuseram
                continue

        # Formata mensagem
        full_msg = subject
        if body:
            # Pega as primeiras linhas do body se relevante
            body_summary = " ".join([l.strip() for l in body.splitlines() if l.strip()][:3])
            if body_summary and len(body_summary) > 5:
                full_msg += f" ({body_summary})"
        commits.append(full_msg)

    # Se não achou commits intermediários, pega os 3 últimos commits não-merge
    if not commits:
        for entry in entries:
            entry = entry.strip()
            if not entry:
                continue
            parts = entry.split("|", 2)
            if len(parts) >= 2:
                subject = parts[1].strip()
                if not subject.startswith("Merge "):
                    commits.append(subject)
            if len(commits) >= 3:
                break

    return commits


def generate_notes_with_ai(mobile_commits: list[str], server_commits: list[str], version: str, build: str) -> dict:
    """Solicita à IA a síntese das alterações no formato padronizado."""
    mobile_text = "\n".join(f"- {c}" for c in mobile_commits) if mobile_commits else "- Otimizações de interface e estabilidade contínua."
    server_text = "\n".join(f"- {c}" for c in server_commits) if server_commits else "- Otimizações de rede, túneis Cloudflare e sincronização."

    prompt = f"""Você é o especialista de documentação e engenharia de software do ecossistema Antigravity (WebuildSoft).
Gere as notas de lançamento oficiais para a versão v{version} (Build #{build}) voltadas para os USUÁRIOS FINAIS.

REPOSITÓRIO MOBILE (Flutter / Android):
{mobile_text}

REPOSITÓRIO SERVIDOR (Go / Companion Desktop):
{server_text}

DIRETRIZES DE FILTRAGEM E ESTILO (CRÍTICO):
1. Foque EXCLUSIVAMENTE no valor entregue ao USUÁRIO: novas funcionalidades, correções de bugs perceptíveis, usabilidade, segurança, velocidade e estabilidade.
2. IGNORE e OMITA sumariamente tarefas internas de infraestrutura do desenvolvedor (ex: alterações em scripts de deploy.sh, automações de build/CI, pipelines locais da máquina ou ajustes de release interna).
3. Se um módulo teve apenas alterações de pipeline/deploy interno, ressalte melhorias de compatibilidade e estabilidade geral (ex: "* **Estabilidade & Conexão:** Sincronização e compatibilidade garantida com a versão mais recente.").
4. Responda ESTRITAMENTE em Português do Brasil de forma concisa, profissional e elegante.
5. Retorne APENAS um objeto JSON válido, sem cercas de markdown (```), sem texto antes ou depois.

ESTRUTURA DO JSON:
{{
  "short_changelog": "Frase resumida e direta (máx 130 caracteres) descrevendo as novidades para os usuários.",
  "mobile_highlights": "Lista formatada em Markdown com bullet points (* **Funcionalidade:** Detalhes) das mudanças no app.",
  "server_highlights": "Lista formatada em Markdown com bullet points (* **Funcionalidade:** Detalhes) das mudanças no servidor companion."
}}"""

    # 1. Tentativa via agy -p (IA nativa do ecossistema Antigravity)
    try:
        agy_cmd = ["agy", "-p", prompt]
        result = subprocess.run(agy_cmd, capture_output=True, text=True, timeout=45)
        if result.returncode == 0 and result.stdout.strip():
            raw = result.stdout.strip()
            # Remove blocos markdown se a IA colocar
            raw = re.sub(r"^```json\s*", "", raw)
            raw = re.sub(r"^```\s*", "", raw)
            raw = re.sub(r"\s*```$", "", raw)
            match = re.search(r"\{[\s\S]*\}", raw)
            if match:
                data = json.loads(match.group(0))
                if "short_changelog" in data and "mobile_highlights" in data and "server_highlights" in data:
                    return data
    except Exception as e:
        sys.stderr.write(f"[WARN] Falha ao invocar IA via agy: {e}\n")

    # 2. Fallback defensivo com base nos próprios commits
    short_summary = "Atualização v" + version + " com melhorias de conectividade e otimizações gerais."
    if mobile_commits:
        short_summary = mobile_commits[0][:120]

    mobile_hl = "\n".join(f"* **Atualização:** {c}" for c in mobile_commits[:5]) if mobile_commits else "* **Melhorias contínuas:** Otimizações gerais de estabilidade e fluidez."
    server_hl = "\n".join(f"* **Atualização:** {c}" for c in server_commits[:5]) if server_commits else "* **Melhorias contínuas:** Otimizações de rede, túneis e sincronização de dados."

    return {
        "short_changelog": short_summary,
        "mobile_highlights": mobile_hl,
        "server_highlights": server_hl,
    }


def main():
    parser = argparse.ArgumentParser(description="Gerador de Release Notes com IA")
    parser.add_argument("--mobile-dir", required=True, help="Caminho do repositório mobile")
    parser.add_argument("--server-dir", required=True, help="Caminho do repositório do servidor")
    parser.add_argument("--version", default="1.4.0", help="Número da versão alvo")
    parser.add_argument("--build", default="1", help="Número do build alvo")
    parser.add_argument("--output-json", default="/tmp/antigravity_release_notes.json", help="Arquivo JSON de saída")
    parser.add_argument("--dry-run", action="store_true", help="Apenas exibe no terminal")

    args = parser.parse_args()

    mobile_commits = get_git_changes(args.mobile_dir, "Mobile")
    server_commits = get_git_changes(args.server_dir, "Server")

    data = generate_notes_with_ai(mobile_commits, server_commits, args.version, args.build)

    if args.dry_run:
        print(json.dumps(data, indent=2, ensure_ascii=False))
        return

    # Garante diretório pai
    os.makedirs(os.path.dirname(os.path.abspath(args.output_json)), exist_ok=True)
    with open(args.output_json, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)

    print(f"OK: Release notes geradas com sucesso em {args.output_json}")


if __name__ == "__main__":
    main()
