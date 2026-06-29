import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  RESUME_SCHEMA_VERSION,
  clearResume,
  readResume,
  resumeKey,
  writeResume,
  type ResumeEntry,
} from "./resumeStore";

const SIG = "deadbeef";

function entry(over: Partial<ResumeEntry> = {}): ResumeEntry {
  return {
    schemaVersion: RESUME_SCHEMA_VERSION,
    blockId: "b002",
    blockOrder: 1,
    blockSignature: SIG,
    updatedAt: 1_000,
    ...over,
  };
}

beforeEach(() => {
  localStorage.clear();
});
afterEach(() => {
  vi.restoreAllMocks();
  localStorage.clear();
});

describe("resumeStore — round-trip + block-level-only persistence", () => {
  it("writes then reads back an identical entry", () => {
    const key = resumeKey("entry:msg-1");
    writeResume(key, entry());
    expect(readResume(key, SIG)).toEqual(entry());
  });

  it("persists NO ms / time-offset field (block-level sync invariant)", () => {
    const key = resumeKey("entry:msg-1");
    writeResume(key, entry());
    const raw = JSON.parse(localStorage.getItem(key)!);
    expect(Object.keys(raw).sort()).toEqual(
      ["blockId", "blockOrder", "blockSignature", "schemaVersion", "updatedAt"].sort(),
    );
    expect(raw).not.toHaveProperty("ms");
    expect(raw).not.toHaveProperty("startMs");
    expect(raw).not.toHaveProperty("currentTime");
  });

  it("resumeKey namespaces under the earshot:resume:v1 prefix", () => {
    expect(resumeKey("entry:abc")).toBe("earshot:resume:v1:entry:abc");
  });
});

describe("resumeStore — validation drops bad / stale entries", () => {
  it("returns null on signature mismatch and self-heals (clears the key)", () => {
    const key = resumeKey("entry:msg-1");
    writeResume(key, entry({ blockSignature: "oldsig00" }));
    expect(readResume(key, SIG)).toBeNull();
    expect(localStorage.getItem(key)).toBeNull(); // cleared
  });

  it("returns null on schemaVersion mismatch", () => {
    const key = resumeKey("entry:msg-1");
    writeResume(key, entry({ schemaVersion: 0 }));
    expect(readResume(key, SIG)).toBeNull();
  });

  it("returns null on malformed JSON", () => {
    const key = resumeKey("entry:msg-1");
    localStorage.setItem(key, "{not json");
    expect(readResume(key, SIG)).toBeNull();
  });

  it("returns null on a shape mismatch (missing fields)", () => {
    const key = resumeKey("entry:msg-1");
    localStorage.setItem(key, JSON.stringify({ blockId: "b002" }));
    expect(readResume(key, SIG)).toBeNull();
  });

  it("ignores the signature check when none is supplied", () => {
    const key = resumeKey("entry:msg-1");
    writeResume(key, entry({ blockSignature: "whatever" }));
    expect(readResume(key)).not.toBeNull();
  });

  it("returns null when the key is absent", () => {
    expect(readResume(resumeKey("entry:nope"), SIG)).toBeNull();
  });
});

describe("resumeStore — storage failures no-op (private mode / quota)", () => {
  it("read returns null when getItem throws", () => {
    vi.spyOn(Storage.prototype, "getItem").mockImplementation(() => {
      throw new Error("SecurityError");
    });
    expect(readResume(resumeKey("entry:x"), SIG)).toBeNull();
  });

  it("write swallows a quota throw without raising", () => {
    vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new Error("QuotaExceededError");
    });
    expect(() => writeResume(resumeKey("entry:x"), entry())).not.toThrow();
  });

  it("clear swallows a throw", () => {
    vi.spyOn(Storage.prototype, "removeItem").mockImplementation(() => {
      throw new Error("nope");
    });
    expect(() => clearResume(resumeKey("entry:x"))).not.toThrow();
  });
});

describe("resumeStore — prune bounds growth (quota guard)", () => {
  it("keeps only the most-recent 20 resume keys on write", () => {
    // Write 25 entries with increasing recency.
    for (let i = 0; i < 25; i++) {
      writeResume(resumeKey(`entry:msg-${i}`), entry({ blockId: `b${i}`, updatedAt: i }));
    }
    const remaining: string[] = [];
    for (let i = 0; i < localStorage.length; i++) {
      const k = localStorage.key(i)!;
      if (k.startsWith("earshot:resume:v1:")) remaining.push(k);
    }
    expect(remaining).toHaveLength(20);
    // The oldest (msg-0) is gone; the newest (msg-24) survives.
    expect(localStorage.getItem(resumeKey("entry:msg-0"))).toBeNull();
    expect(localStorage.getItem(resumeKey("entry:msg-24"))).not.toBeNull();
  });
});
