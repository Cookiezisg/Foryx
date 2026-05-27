// errorMap — unit tests for errorKey + kindForCode.

import { describe, expect, it } from "vitest";
import { errorKey, kindForCode } from "./errorMap";

describe("errorKey", () => {
  it("known_code_CONVERSATION_NOT_FOUND_returnsKey", () => {
    expect(errorKey("CONVERSATION_NOT_FOUND")).toBe("errors:CONVERSATION_NOT_FOUND");
  });

  it("known_code_INTERNAL_ERROR_returnsKey", () => {
    expect(errorKey("INTERNAL_ERROR")).toBe("errors:INTERNAL_ERROR");
  });

  it("known_code_UNAUTH_NO_USER_returnsKey", () => {
    expect(errorKey("UNAUTH_NO_USER")).toBe("errors:UNAUTH_NO_USER");
  });

  it("known_code_LLM_PROVIDER_ERROR_returnsKey", () => {
    expect(errorKey("LLM_PROVIDER_ERROR")).toBe("errors:LLM_PROVIDER_ERROR");
  });

  it("known_code_NETWORK_returnsKey", () => {
    expect(errorKey("NETWORK")).toBe("errors:NETWORK");
  });

  it("unknown_code_returnsFallbackKey", () => {
    expect(errorKey("TOTALLY_UNKNOWN")).toBe("errors:fallback");
  });

  it("empty_string_returnsFallbackKey", () => {
    expect(errorKey("")).toBe("errors:fallback");
  });
});

describe("kindForCode", () => {
  it("CONVERSATION_NOT_FOUND_returnsWarn", () => {
    expect(kindForCode("CONVERSATION_NOT_FOUND")).toBe("warn");
  });

  it("INTERNAL_ERROR_returnsError", () => {
    expect(kindForCode("INTERNAL_ERROR")).toBe("error");
  });

  it("anyOtherCode_returnsError", () => {
    expect(kindForCode("LLM_RATE_LIMITED")).toBe("error");
    expect(kindForCode("UNKNOWN")).toBe("error");
  });
});
