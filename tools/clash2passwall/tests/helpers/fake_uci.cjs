#!/usr/bin/env node
const fs = require("fs");
const path = require("path");

const args = process.argv.slice(2);
let configDir = null;
let quiet = false;
for (let index = 0; index < args.length;) {
  if (args[index] === "-c") {
    configDir = args[index + 1];
    args.splice(index, 2);
  } else if (args[index] === "-q") {
    quiet = true;
    args.splice(index, 1);
  } else {
    index += 1;
  }
}

const configFile = configDir
  ? path.join(configDir, "passwall2")
  : process.env.FAKE_UCI_LIVE_CONF;
const fail = (message) => {
  if (!quiet && message) process.stderr.write(message + "\n");
  process.exit(1);
};
if (!configFile) fail("FAKE_UCI_LIVE_CONF is required");

function blocks() {
  const text = fs.readFileSync(configFile, "utf8");
  return text.match(/^config\s[^\n]+(?:\n(?!config\s)[^\n]*)*\n?/gm) || [];
}

function metadata(block) {
  const match = block.match(/^config\s+(\S+)(?:\s+'([^']+)')?/);
  return { type: match && match[1], name: match && match[2] };
}

function resolveTarget(target) {
  const match = target.match(/^passwall2\.(?:@([A-Za-z0-9_]+)\[(\d+)\]|([A-Za-z0-9_]+))(?:\.([A-Za-z0-9_]+))?$/);
  if (!match) fail("unsupported target: " + target);
  const list = blocks();
  let index;
  if (match[1]) {
    const matches = list.map((block, blockIndex) => [metadata(block), blockIndex]).filter(([meta]) => meta.type === match[1]);
    index = matches[Number(match[2])] && matches[Number(match[2])][1];
  } else {
    index = list.findIndex((block) => metadata(block).name === match[3]);
  }
  return { list, index, option: match[4] };
}

function write(list) {
  fs.writeFileSync(configFile, list.join(""), "utf8");
}

const command = args.shift();
if (command === "changes") process.exit(0);
if (command === "commit") {
  if ((configDir && process.env.FAKE_UCI_FAIL_STAGE_COMMIT === "1") || (!configDir && process.env.FAKE_UCI_FAIL_LIVE_COMMIT === "1")) fail("injected commit failure");
  process.exit(0);
}
if (command === "show") {
  if (args[0] && args[0] !== "passwall2") {
    const resolved = resolveTarget(args[0]);
    if (resolved.index == null || resolved.index < 0) fail();
    const meta = metadata(resolved.list[resolved.index]);
    process.stdout.write(`passwall2.${meta.name}=${meta.type}\n`);
    process.exit(0);
  }
  const names = new Set();
  for (const block of blocks()) {
    const meta = metadata(block);
    if (!meta.name) fail("anonymous section rejected by fake UCI contract");
    if (names.has(meta.name)) fail("duplicate section name rejected by fake UCI contract");
    names.add(meta.name);
    process.stdout.write(`passwall2.${meta.name}=${meta.type}\n`);
  }
  process.exit(0);
}
if (command === "set") {
  const assignment = args.join(" ");
  const equals = assignment.indexOf("=");
  const target = assignment.slice(0, equals);
  const value = assignment.slice(equals + 1);
  const resolved = resolveTarget(target);
  if (resolved.index == null || resolved.index < 0 || !resolved.option) fail("set target missing");
  const optionPattern = new RegExp(`^([ \\t]*option[ \\t]+${resolved.option}[ \\t]+).*$`, "m");
  const escaped = value.replace(/'/g, "'\\''");
  if (optionPattern.test(resolved.list[resolved.index])) {
    resolved.list[resolved.index] = resolved.list[resolved.index].replace(optionPattern, `$1'${escaped}'`);
  } else {
    resolved.list[resolved.index] = resolved.list[resolved.index].replace(/\n?$/, `\n\toption ${resolved.option} '${escaped}'\n`);
  }
  write(resolved.list);
  process.exit(0);
}
if (command === "delete") {
  const resolved = resolveTarget(args[0]);
  if (resolved.index == null || resolved.index < 0) fail();
  if (resolved.option) {
    const pattern = new RegExp(`^[ \\t]*option[ \\t]+${resolved.option}[ \\t]+.*\\n?`, "m");
    resolved.list[resolved.index] = resolved.list[resolved.index].replace(pattern, "");
  } else {
    resolved.list.splice(resolved.index, 1);
  }
  write(resolved.list);
  process.exit(0);
}
if (command === "get") {
  const resolved = resolveTarget(args[0]);
  if (resolved.index == null || resolved.index < 0 || !resolved.option) fail();
  const match = resolved.list[resolved.index].match(new RegExp(`^[ \\t]*option[ \\t]+${resolved.option}[ \\t]+'([^']*)'`, "m"));
  if (!match) fail();
  process.stdout.write(match[1] + "\n");
  process.exit(0);
}
fail("unsupported command: " + command);
