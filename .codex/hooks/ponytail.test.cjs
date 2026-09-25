const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const { spawnSync } = require('node:child_process');

test('project hooks load local skills and keep state local', () => {
  const temp = fs.mkdtempSync(path.join(os.tmpdir(), 'ponytail-'));
  try {
    const root = path.join(temp, 'project with spaces');
    fs.cpSync(path.resolve(__dirname, '..'), path.join(root, '.codex'), { recursive: true });
    fs.cpSync(path.resolve(__dirname, '../../.agents/skills/ponytail'), path.join(root, '.agents/skills/ponytail'), { recursive: true });
    assert.equal(spawnSync('git', ['init', '-q', root]).status, 0);
    const cwd = path.join(root, 'nested');
    fs.mkdirSync(cwd);
    const config = require('../hooks.json');
    const env = { ...process.env, PONYTAIL_DEFAULT_MODE: '', PONYTAIL_SUBAGENT_MATCHER: '' };
    function run(event, prompt = '') {
      const command = config.hooks[event][0].hooks[0].command;
      const result = spawnSync(command, {
        shell: true, cwd, env, input: JSON.stringify({ prompt }), encoding: 'utf8', timeout: 5000,
      });
      assert.equal(result.status, 0, result.stderr);
      return result.stdout ? JSON.parse(result.stdout) : null;
    }
    assert.equal(run('SessionStart').systemMessage, 'PONYTAIL:FULL');
    assert.match(run('SubagentStart').hookSpecificOutput.additionalContext, /## Intensity/);
    assert.equal(run('UserPromptSubmit', '$ponytail lite').systemMessage, 'PONYTAIL:LITE');
    assert.equal(run('SubagentStart').systemMessage, 'PONYTAIL:LITE');
    run('UserPromptSubmit', '$ponytail off');
    assert.equal(run('SubagentStart'), null);
    run('UserPromptSubmit', '$ponytail default ultra');
    assert.equal(run('SessionStart').systemMessage, 'PONYTAIL:ULTRA');
    assert.equal(fs.readFileSync(path.join(root, '.codex/ponytail-state/.ponytail-active'), 'utf8'), 'ultra');
    const saved = JSON.parse(fs.readFileSync(path.join(root, '.codex/ponytail-config/ponytail/config.json')));
    assert.equal(saved.defaultMode, 'ultra');
  } finally {
    fs.rmSync(temp, { recursive: true, force: true });
  }
});
