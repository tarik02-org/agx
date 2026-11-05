import { createRequire } from "node:module";
import { define } from "@opencode/plugin/effect/plugin";
import * as Effect from "effect/Effect";
import { effectify } from "effect/Effect";
import * as Layer from "effect/Layer";
import * as Path$2 from "effect/Path";
import { Path, make } from "effect/Path";
import * as Schedule from "effect/Schedule";
import * as Schema$1 from "effect/Schema";
import * as Context from "effect/Context";
import * as Config from "effect/Config";
import * as PlatformError from "effect/PlatformError";
import { BadArgument, badArgument, systemError } from "effect/PlatformError";
import * as FileSystem from "effect/FileSystem";
import * as ChildProcess from "effect/process/ChildProcess";
import * as ChildProcessSpawner$1 from "effect/process/ChildProcessSpawner";
import { ChildProcessSpawner, ExitCode, ProcessId, make as make$1, makeHandle } from "effect/process/ChildProcessSpawner";
import * as Clock from "effect/Clock";
import * as Crypto from "effect/Crypto";
import * as Hex from "effect/encoding/Hex";
import * as Option$1 from "effect/Option";
import * as ScopedRef from "effect/ScopedRef";
import * as Semaphore from "effect/Semaphore";
import { DateTime, Option, Schema, SchemaGetter, Struct } from "effect";
import * as Scope from "effect/Scope";
import * as Deferred from "effect/Deferred";
import * as Duration from "effect/Duration";
import * as Exit from "effect/Exit";
import * as Predicate from "effect/Predicate";
import * as Sink from "effect/Sink";
import * as Stream from "effect/Stream";
import * as NodeChildProcess from "node:child_process";
import { PassThrough, finished } from "node:stream";
import * as Cause from "effect/Cause";
import * as Channel from "effect/Channel";
import { constVoid, identity, pipe } from "effect/Function";
import * as Pull from "effect/Pull";
import * as Arr from "effect/Array";
import * as ByteSize from "effect/ByteSize";
import "effect/Fiber";
import * as Latch from "effect/Latch";
import * as MutableRef from "effect/MutableRef";
import * as NodeCrypto from "node:crypto";
import * as BI from "effect/BigInt";
import * as Queue from "effect/Queue";
import * as NFS from "node:fs";
import * as OS from "node:os";
import * as Path$1 from "node:path";
import * as NodeUrl from "node:url";
import * as Stdio from "effect/Stdio";
import * as RcRef from "effect/RcRef";
import * as Terminal from "effect/Terminal";
import * as readline from "node:readline";
// \0rolldown/runtime.js
var __commonJSMin = (cb, mod) => () => (mod || (cb((mod = { exports: {} }).exports, mod, mod.exports), cb = null), mod.exports);
var __require = /* #__PURE__ */ (() => createRequire(import.meta.url))();
// src/Configuration.ts
var Configuration = class extends Context.Service()("agx-opencode/Configuration") {};
// src/error.ts
var IntegrationError = class extends Schema$1.TaggedError()("IntegrationError", {
	operation: Schema$1.String,
	reason: Schema$1.Literals([
		"options",
		"bootstrap",
		"discovery",
		"content",
		"filesystem",
		"settings"
	]),
	cause: Schema$1.optionalKey(Schema$1.Union([
		Schema$1.instanceOf(PlatformError.PlatformError),
		Schema$1.instanceOf(Config.ConfigError),
		Schema$1.instanceOf(Schema$1.SchemaError),
		Schema$1.Struct({
			kind: Schema$1.Literal("yaml"),
			code: Schema$1.String
		})
	]))
}) {
	get message() {
		return `AGX ${this.operation} failed (${this.reason})`;
	}
};
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/code.js
/**
* Connect represents categories of errors as codes, and each code maps to a
* specific HTTP status code. The codes and their semantics were chosen to
* match gRPC. Only the codes below are valid — there are no user-defined
* codes.
*
* See the specification at https://connectrpc.com/docs/protocol#error-codes
* for details.
*/
var Code;
(function(Code) {
	/**
	* Canceled, usually by the user
	*/
	Code[Code["Canceled"] = 1] = "Canceled";
	/**
	* Unknown error
	*/
	Code[Code["Unknown"] = 2] = "Unknown";
	/**
	* Argument invalid regardless of system state
	*/
	Code[Code["InvalidArgument"] = 3] = "InvalidArgument";
	/**
	* Operation expired, may or may not have completed.
	*/
	Code[Code["DeadlineExceeded"] = 4] = "DeadlineExceeded";
	/**
	* Entity not found.
	*/
	Code[Code["NotFound"] = 5] = "NotFound";
	/**
	* Entity already exists.
	*/
	Code[Code["AlreadyExists"] = 6] = "AlreadyExists";
	/**
	* Operation not authorized.
	*/
	Code[Code["PermissionDenied"] = 7] = "PermissionDenied";
	/**
	* Quota exhausted.
	*/
	Code[Code["ResourceExhausted"] = 8] = "ResourceExhausted";
	/**
	* Argument invalid in current system state.
	*/
	Code[Code["FailedPrecondition"] = 9] = "FailedPrecondition";
	/**
	* Operation aborted.
	*/
	Code[Code["Aborted"] = 10] = "Aborted";
	/**
	* Out of bounds, use instead of FailedPrecondition.
	*/
	Code[Code["OutOfRange"] = 11] = "OutOfRange";
	/**
	* Operation not implemented or disabled.
	*/
	Code[Code["Unimplemented"] = 12] = "Unimplemented";
	/**
	* Internal error, reserved for "serious errors".
	*/
	Code[Code["Internal"] = 13] = "Internal";
	/**
	* Unavailable, client should back off and retry.
	*/
	Code[Code["Unavailable"] = 14] = "Unavailable";
	/**
	* Unrecoverable data loss or corruption.
	*/
	Code[Code["DataLoss"] = 15] = "DataLoss";
	/**
	* Request isn't authenticated.
	*/
	Code[Code["Unauthenticated"] = 16] = "Unauthenticated";
})(Code || (Code = {}));
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/is-message.js
/**
* Determine whether the given `arg` is a message.
* If `desc` is set, determine whether `arg` is this specific message.
*/
function isMessage(arg, schema) {
	if (!(arg !== null && typeof arg == "object" && "$typeName" in arg && typeof arg.$typeName == "string")) return false;
	if (schema === void 0) return true;
	return schema.typeName === arg.$typeName;
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/descriptors.js
/**
* Scalar value types. This is a subset of field types declared by protobuf
* enum google.protobuf.FieldDescriptorProto.Type The types GROUP and MESSAGE
* are omitted, but the numerical values are identical.
*/
var ScalarType;
(function(ScalarType) {
	ScalarType[ScalarType["DOUBLE"] = 1] = "DOUBLE";
	ScalarType[ScalarType["FLOAT"] = 2] = "FLOAT";
	ScalarType[ScalarType["INT64"] = 3] = "INT64";
	ScalarType[ScalarType["UINT64"] = 4] = "UINT64";
	ScalarType[ScalarType["INT32"] = 5] = "INT32";
	ScalarType[ScalarType["FIXED64"] = 6] = "FIXED64";
	ScalarType[ScalarType["FIXED32"] = 7] = "FIXED32";
	ScalarType[ScalarType["BOOL"] = 8] = "BOOL";
	ScalarType[ScalarType["STRING"] = 9] = "STRING";
	ScalarType[ScalarType["BYTES"] = 12] = "BYTES";
	ScalarType[ScalarType["UINT32"] = 13] = "UINT32";
	ScalarType[ScalarType["SFIXED32"] = 15] = "SFIXED32";
	ScalarType[ScalarType["SFIXED64"] = 16] = "SFIXED64";
	ScalarType[ScalarType["SINT32"] = 17] = "SINT32";
	ScalarType[ScalarType["SINT64"] = 18] = "SINT64";
})(ScalarType || (ScalarType = {}));
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/wire/varint.js
/**
* Read a 64 bit varint as two JS numbers.
*
* Stores the low and high words on the reader.
*
* Copyright 2008 Google Inc.  All rights reserved.
*
* See https://github.com/protocolbuffers/protobuf/blob/8a71927d74a4ce34efe2d8769fda198f52d20d12/js/experimental/runtime/kernel/buffer_decoder.js#L175
*/
function varint64read() {
	const buf = this.buf;
	let pos = this.pos;
	let lo = 0;
	let hi = 0;
	for (let shift = 0; shift < 28; shift += 7) {
		const b = buf[pos++];
		lo |= (b & 127) << shift;
		if ((b & 128) == 0) {
			this.pos = pos;
			this.assertBounds();
			this.varint64Lo = lo;
			this.varint64Hi = hi;
			return;
		}
	}
	const middleByte = buf[pos++];
	lo |= (middleByte & 15) << 28;
	hi = (middleByte & 112) >> 4;
	if ((middleByte & 128) == 0) {
		this.pos = pos;
		this.assertBounds();
		this.varint64Lo = lo;
		this.varint64Hi = hi;
		return;
	}
	for (let shift = 3; shift <= 31; shift += 7) {
		const b = buf[pos++];
		hi |= (b & 127) << shift;
		if ((b & 128) == 0) {
			this.pos = pos;
			this.assertBounds();
			this.varint64Lo = lo;
			this.varint64Hi = hi;
			return;
		}
	}
	throw new Error("invalid varint");
}
const TWO_PWR_32_DBL = 4294967296;
/**
* Parse decimal string of 64 bit integer value as two JS numbers.
*
* Copyright 2008 Google Inc.  All rights reserved.
*
* See https://github.com/protocolbuffers/protobuf-javascript/blob/a428c58273abad07c66071d9753bc4d1289de426/experimental/runtime/int64.js#L10
*/
function int64FromString(dec) {
	const minus = dec[0] === "-";
	if (minus) dec = dec.slice(1);
	const base = 1e6;
	let lowBits = 0;
	let highBits = 0;
	function add1e6digit(begin, end) {
		const digit1e6 = Number(dec.slice(begin, end));
		highBits *= base;
		lowBits = lowBits * base + digit1e6;
		if (lowBits >= TWO_PWR_32_DBL) {
			highBits = highBits + (lowBits / TWO_PWR_32_DBL | 0);
			lowBits = lowBits % TWO_PWR_32_DBL;
		}
	}
	add1e6digit(-24, -18);
	add1e6digit(-18, -12);
	add1e6digit(-12, -6);
	add1e6digit(-6);
	return minus ? negate(lowBits, highBits) : newBits(lowBits, highBits);
}
/**
* Losslessly converts a 64-bit signed integer in 32:32 split representation
* into a decimal string.
*
* Copyright 2008 Google Inc.  All rights reserved.
*
* See https://github.com/protocolbuffers/protobuf-javascript/blob/a428c58273abad07c66071d9753bc4d1289de426/experimental/runtime/int64.js#L10
*/
function int64ToString(lo, hi) {
	let bits = newBits(lo, hi);
	const negative = bits.hi & 2147483648;
	if (negative) bits = negate(bits.lo, bits.hi);
	const result = uInt64ToString(bits.lo, bits.hi);
	return negative ? "-" + result : result;
}
/**
* Losslessly converts a 64-bit unsigned integer in 32:32 split representation
* into a decimal string.
*
* Copyright 2008 Google Inc.  All rights reserved.
*
* See https://github.com/protocolbuffers/protobuf-javascript/blob/a428c58273abad07c66071d9753bc4d1289de426/experimental/runtime/int64.js#L10
*/
function uInt64ToString(lo, hi) {
	({lo, hi} = toUnsigned(lo, hi));
	if (hi <= 2097151) return String(TWO_PWR_32_DBL * hi + lo);
	const low = lo & 16777215;
	const mid = (lo >>> 24 | hi << 8) & 16777215;
	const high = hi >> 16 & 65535;
	let digitA = low + mid * 6777216 + high * 6710656;
	let digitB = mid + high * 8147497;
	let digitC = high * 2;
	const base = 1e7;
	if (digitA >= base) {
		digitB += Math.floor(digitA / base);
		digitA %= base;
	}
	if (digitB >= base) {
		digitC += Math.floor(digitB / base);
		digitB %= base;
	}
	return digitC.toString() + decimalFrom1e7WithLeadingZeros(digitB) + decimalFrom1e7WithLeadingZeros(digitA);
}
function toUnsigned(lo, hi) {
	return {
		lo: lo >>> 0,
		hi: hi >>> 0
	};
}
function newBits(lo, hi) {
	return {
		lo: lo | 0,
		hi: hi | 0
	};
}
/**
* Returns two's compliment negation of input.
* @see https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Operators/Bitwise_Operators#Signed_32-bit_integers
*/
function negate(lowBits, highBits) {
	highBits = ~highBits;
	if (lowBits) lowBits = ~lowBits + 1;
	else highBits += 1;
	return newBits(lowBits, highBits);
}
/**
* Returns decimal representation of digit1e7 with leading zeros.
*/
const decimalFrom1e7WithLeadingZeros = (digit1e7) => {
	const partial = String(digit1e7);
	return "0000000".slice(partial.length) + partial;
};
/**
* Write a 32 bit varint, signed or unsigned. Same as `varint64write(0, value, bytes)`
*
* Copyright 2008 Google Inc.  All rights reserved.
*
* See https://github.com/protocolbuffers/protobuf/blob/1b18833f4f2a2f681f4e4a25cdf3b0a43115ec26/js/binary/encoder.js#L144
*/
function varint32write(value, bytes) {
	if (value >>> 0 < 128) {
		bytes.push(value);
		return;
	}
	if (value >= 0) {
		while (value > 127) {
			bytes.push(value & 127 | 128);
			value = value >>> 7;
		}
		bytes.push(value);
	} else {
		for (let i = 0; i < 9; i++) {
			bytes.push(value & 127 | 128);
			value = value >> 7;
		}
		bytes.push(1);
	}
}
/**
* Read an unsigned 32 bit varint.
*
* See https://github.com/protocolbuffers/protobuf/blob/8a71927d74a4ce34efe2d8769fda198f52d20d12/js/experimental/runtime/kernel/buffer_decoder.js#L220
*/
function varint32read() {
	let b = this.buf[this.pos++];
	if ((b & 128) === 0) {
		this.assertBounds();
		return b;
	}
	let result = b & 127;
	b = this.buf[this.pos++];
	result |= (b & 127) << 7;
	if ((b & 128) === 0) {
		this.assertBounds();
		return result;
	}
	b = this.buf[this.pos++];
	result |= (b & 127) << 14;
	if ((b & 128) === 0) {
		this.assertBounds();
		return result;
	}
	b = this.buf[this.pos++];
	result |= (b & 127) << 21;
	if ((b & 128) === 0) {
		this.assertBounds();
		return result;
	}
	b = this.buf[this.pos++];
	result |= (b & 15) << 28;
	for (let readBytes = 5; (b & 128) !== 0 && readBytes < 10; readBytes++) b = this.buf[this.pos++];
	if ((b & 128) !== 0) throw new Error("invalid varint");
	this.assertBounds();
	return result >>> 0;
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/proto-int64.js
/**
* Int64Support for the current environment.
*/
const protoInt64 = /*@__PURE__*/ makeInt64Support();
function makeInt64Support() {
	const dv = /* @__PURE__ */ new DataView(/* @__PURE__ */ new ArrayBuffer(8));
	if (typeof BigInt === "function" && typeof dv.getBigInt64 === "function" && typeof dv.getBigUint64 === "function" && typeof dv.setBigInt64 === "function" && typeof dv.setBigUint64 === "function" && (!!globalThis.Deno || !!globalThis.Bun || typeof process != "object" || typeof process.env != "object" || process.env.BUF_BIGINT_DISABLE !== "1")) {
		const MIN = BigInt("-9223372036854775808");
		const MAX = BigInt("9223372036854775807");
		const UMIN = BigInt("0");
		const UMAX = BigInt("18446744073709551615");
		return {
			zero: BigInt(0),
			supported: true,
			parse(value) {
				const bi = typeof value == "bigint" ? value : BigInt(value);
				if (bi > MAX || bi < MIN) throw new Error(`invalid int64: ${value}`);
				return bi;
			},
			uParse(value) {
				const bi = typeof value == "bigint" ? value : BigInt(value);
				if (bi > UMAX || bi < UMIN) throw new Error(`invalid uint64: ${value}`);
				return bi;
			},
			enc(value) {
				dv.setBigInt64(0, this.parse(value), true);
				return {
					lo: dv.getInt32(0, true),
					hi: dv.getInt32(4, true)
				};
			},
			uEnc(value) {
				dv.setBigInt64(0, this.uParse(value), true);
				return {
					lo: dv.getInt32(0, true),
					hi: dv.getInt32(4, true)
				};
			},
			dec(lo, hi) {
				dv.setInt32(0, lo, true);
				dv.setInt32(4, hi, true);
				return dv.getBigInt64(0, true);
			},
			uDec(lo, hi) {
				dv.setInt32(0, lo, true);
				dv.setInt32(4, hi, true);
				return dv.getBigUint64(0, true);
			}
		};
	}
	return {
		zero: "0",
		supported: false,
		parse(value) {
			if (typeof value != "string") value = value.toString();
			assertInt64String(value);
			return value;
		},
		uParse(value) {
			if (typeof value != "string") value = value.toString();
			assertUInt64String(value);
			return value;
		},
		enc(value) {
			if (typeof value != "string") value = value.toString();
			assertInt64String(value);
			return int64FromString(value);
		},
		uEnc(value) {
			if (typeof value != "string") value = value.toString();
			assertUInt64String(value);
			return int64FromString(value);
		},
		dec(lo, hi) {
			return int64ToString(lo, hi);
		},
		uDec(lo, hi) {
			return uInt64ToString(lo, hi);
		}
	};
}
function assertInt64String(value) {
	if (!/^-?[0-9]+$/.test(value)) throw new Error("invalid int64: " + value);
}
function assertUInt64String(value) {
	if (!/^[0-9]+$/.test(value)) throw new Error("invalid uint64: " + value);
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/scalar.js
/**
* Returns the zero value for the given scalar type, the value a field of this
* type has when unset: 0 for numeric types, "" for strings, false for
* booleans, and an empty Uint8Array for bytes. For 64-bit integer types, the
* result is "0" when longAsString is true, otherwise 0n.
*
* This is the type's zero value, not a proto2 custom field default. For float
* and double it is +0; isScalarZeroValue treats only +0, not -0, as this value.
*/
function scalarZeroValue(type, longAsString) {
	switch (type) {
		case ScalarType.STRING: return "";
		case ScalarType.BOOL: return false;
		case ScalarType.DOUBLE:
		case ScalarType.FLOAT: return 0;
		case ScalarType.INT64:
		case ScalarType.UINT64:
		case ScalarType.SFIXED64:
		case ScalarType.FIXED64:
		case ScalarType.SINT64: return longAsString ? "0" : protoInt64.zero;
		case ScalarType.BYTES: return /* @__PURE__ */ new Uint8Array(0);
		default: return 0;
	}
}
/**
* Returns true if the value is the zero value for the given scalar type: `0`
* for numeric types, `false` for booleans, `""` for strings, and an empty
* Uint8Array for bytes.
*
* This is the implicit-presence default check. A singular field with implicit
* presence is treated as unset, and omitted from the wire, when its value is
* the zero value. With explicit presence, or in repeated and map fields,
* presence is structural and this function does not apply.
*
* Note that -0 is NOT a zero value for float and double: under implicit
* presence, +0 is omitted from the wire but -0 is written, following the
* proto3 specification. As a result this can disagree with scalarEquals, which
* compares by value and treats -0 as equal to 0.
*/
function isScalarZeroValue(type, value) {
	switch (type) {
		case ScalarType.BOOL: return value === false;
		case ScalarType.STRING: return value === "";
		case ScalarType.BYTES: return value instanceof Uint8Array && !value.byteLength;
		case ScalarType.DOUBLE:
		case ScalarType.FLOAT: return Object.is(value, 0);
		default: return value == 0;
	}
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/unsafe.js
const IMPLICIT$5 = 2;
const unsafeLocal = Symbol.for("reflect unsafe local");
/**
* Return the selected field of a oneof group.
*
* @private
*/
function unsafeOneofCase(target, oneof) {
	const c = target[oneof.localName].case;
	if (c === void 0) return c;
	return oneof.fields.find((f) => f.localName === c);
}
/**
* Returns true if the field is set.
*
* @private
*/
function unsafeIsSet(target, field) {
	const name = field.localName;
	if (field.oneof) return target[field.oneof.localName].case === name;
	if (field.presence != IMPLICIT$5) return target[name] !== void 0 && Object.prototype.hasOwnProperty.call(target, name);
	switch (field.fieldKind) {
		case "list": return target[name].length > 0;
		case "map": return Object.keys(target[name]).length > 0;
		case "scalar": return !isScalarZeroValue(field.scalar, target[name]);
		case "enum": return target[name] !== field.enum.values[0].number;
	}
	throw new Error("message field with implicit presence");
}
/**
* Returns true if the field is set, but only for singular fields with explicit
* presence (proto2).
*
* @private
*/
function unsafeIsSetExplicit(target, localName) {
	return Object.prototype.hasOwnProperty.call(target, localName) && target[localName] !== void 0;
}
/**
* Return a field value, respecting oneof groups.
*
* @private
*/
function unsafeGet(target, field) {
	if (field.oneof) {
		const oneof = target[field.oneof.localName];
		if (oneof.case === field.localName) return oneof.value;
		return;
	}
	return target[field.localName];
}
/**
* Set a field value, respecting oneof groups.
*
* @private
*/
function unsafeSet(target, field, value) {
	if (field.oneof) target[field.oneof.localName] = {
		case: field.localName,
		value
	};
	else target[field.localName] = value;
}
/**
* Set a property, treating __proto__ as an own data property.
*
* @private
*/
function setOwn(obj, key, value) {
	if (key === "__proto__") Object.defineProperty(obj, "__proto__", {
		value,
		writable: true,
		enumerable: true,
		configurable: true
	});
	else obj[key] = value;
}
/**
* Get a property, disregarding inherited members.
*
* @private
*/
function getOwn(obj, key) {
	return Object.prototype.hasOwnProperty.call(obj, key) ? obj[key] : void 0;
}
/**
* Resets the field, so that unsafeIsSet() will return false.
*
* @private
*/
function unsafeClear(target, field) {
	const name = field.localName;
	if (field.oneof) {
		const oneofLocalName = field.oneof.localName;
		if (target[oneofLocalName].case === name) target[oneofLocalName] = { case: void 0 };
	} else if (field.presence != IMPLICIT$5) delete target[name];
	else switch (field.fieldKind) {
		case "map":
			target[name] = {};
			break;
		case "list":
			target[name] = [];
			break;
		case "enum":
			target[name] = field.enum.values[0].number;
			break;
		case "scalar": target[name] = scalarZeroValue(field.scalar, field.longAsString);
	}
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/guard.js
function isObject(arg) {
	return arg !== null && typeof arg == "object" && !Array.isArray(arg);
}
function isReflectList(arg, field) {
	var _a, _b, _c, _d;
	if (isObject(arg) && unsafeLocal in arg && "add" in arg && "field" in arg && typeof arg.field == "function") {
		if (field !== void 0) {
			const a = field;
			const b = arg.field();
			return a.listKind == b.listKind && a.scalar === b.scalar && ((_a = a.message) === null || _a === void 0 ? void 0 : _a.typeName) === ((_b = b.message) === null || _b === void 0 ? void 0 : _b.typeName) && ((_c = a.enum) === null || _c === void 0 ? void 0 : _c.typeName) === ((_d = b.enum) === null || _d === void 0 ? void 0 : _d.typeName);
		}
		return true;
	}
	return false;
}
function isReflectMap(arg, field) {
	var _a, _b, _c, _d;
	if (isObject(arg) && unsafeLocal in arg && "has" in arg && "field" in arg && typeof arg.field == "function") {
		if (field !== void 0) {
			const a = field, b = arg.field();
			return a.mapKey === b.mapKey && a.mapKind == b.mapKind && a.scalar === b.scalar && ((_a = a.message) === null || _a === void 0 ? void 0 : _a.typeName) === ((_b = b.message) === null || _b === void 0 ? void 0 : _b.typeName) && ((_c = a.enum) === null || _c === void 0 ? void 0 : _c.typeName) === ((_d = b.enum) === null || _d === void 0 ? void 0 : _d.typeName);
		}
		return true;
	}
	return false;
}
function isReflectMessage(arg, messageDesc) {
	return isObject(arg) && unsafeLocal in arg && "desc" in arg && isObject(arg.desc) && arg.desc.kind === "message" && (messageDesc === void 0 || arg.desc.typeName == messageDesc.typeName);
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/wrappers.js
function isWrapper(arg) {
	return isWrapperTypeName(arg.$typeName);
}
function isWrapperDesc(messageDesc) {
	const f = messageDesc.fields[0];
	return isWrapperTypeName(messageDesc.typeName) && f !== void 0 && f.fieldKind == "scalar" && f.name == "value" && f.number == 1;
}
/**
* Returns true if the descriptor is a well-known type with a custom JSON
* representation per the protobuf JSON spec. Examples: Timestamp as an
* RFC 3339 string, Duration as "5s", wrappers as the unwrapped scalar.
*
* When packed inside `google.protobuf.Any`, these messages are serialized
* as `{"@type": ..., "value": <custom form>}`; all other messages embed
* their fields directly.
*/
function hasCustomJsonRepresentation(desc) {
	switch (desc.typeName) {
		case "google.protobuf.Any":
		case "google.protobuf.Timestamp":
		case "google.protobuf.Duration":
		case "google.protobuf.FieldMask":
		case "google.protobuf.Struct":
		case "google.protobuf.Value":
		case "google.protobuf.ListValue": return true;
		default: return isWrapperDesc(desc);
	}
}
const wrapperTypeNames = /*@__PURE__*/ new Set([
	"google.protobuf.DoubleValue",
	"google.protobuf.FloatValue",
	"google.protobuf.Int64Value",
	"google.protobuf.UInt64Value",
	"google.protobuf.Int32Value",
	"google.protobuf.UInt32Value",
	"google.protobuf.BoolValue",
	"google.protobuf.StringValue",
	"google.protobuf.BytesValue"
]);
function isWrapperTypeName(name) {
	return wrapperTypeNames.has(name);
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/create.js
const EDITION_PROTO3$1 = 999;
const EDITION_PROTO2$1 = 998;
const IMPLICIT$4 = 2;
/**
* Create a new message instance.
*
* The second argument is an optional initializer object, where all fields are
* optional.
*/
function create$1(schema, init) {
	if (isMessage(init, schema)) return init;
	return compiledCreate(schema)(init);
}
const compiledCreates = /* @__PURE__ */ new WeakMap();
/**
* Return the compiled create function for a message, compiling it on first use. */
function compiledCreate(desc) {
	let compiled = compiledCreates.get(desc);
	if (compiled === void 0) {
		compiled = compileCreate(desc);
		compiledCreates.set(desc, compiled);
	}
	return compiled;
}
/** Singular field: scalar, enum, or message. */
const INIT_SINGULAR = 0;
/** List field: a zero message has a fresh empty array. */
const INIT_LIST = 1;
/** Map field: a zero message has a fresh empty object. */
const INIT_MAP = 2;
/** Oneof group: the ADT is always stored, cases convert by case name. */
const INIT_ONEOF = 3;
function compileCreate(desc) {
	const typeName = desc.typeName;
	const { properties, prototype } = compileInitMessage(desc);
	return (init) => {
		let message;
		if (prototype !== void 0) {
			message = Object.create(prototype);
			message.$typeName = typeName;
		} else message = { $typeName: typeName };
		for (let i = 0; i < properties.length; i++) {
			const property = properties[i];
			const name = property.name;
			const initValue = init === null || init === void 0 ? void 0 : init[name];
			switch (property.kind) {
				case INIT_SINGULAR:
					if (initValue != null) message[name] = property.convert !== void 0 ? property.convert(initValue) : initValue;
					else if (property.constant !== void 0) message[name] = property.constant;
					break;
				case INIT_LIST:
					message[name] = property.convert !== void 0 && Array.isArray(initValue) ? initValue.map(property.convert) : initValue !== null && initValue !== void 0 ? initValue : [];
					break;
				case INIT_MAP:
					if (property.convert === void 0 || !isObject(initValue)) message[name] = initValue !== null && initValue !== void 0 ? initValue : {};
					else {
						const converted = {};
						const keys = Object.keys(initValue);
						for (let k = 0; k < keys.length; k++) setOwn(converted, keys[k], property.convert(initValue[keys[k]]));
						message[name] = converted;
					}
					break;
				case INIT_ONEOF: {
					const oneofValue = initValue;
					if ((oneofValue === null || oneofValue === void 0 ? void 0 : oneofValue.case) != null) {
						const convert = property.convert.get(oneofValue.case);
						if (convert !== void 0) {
							message[name] = {
								case: oneofValue.case,
								value: convert(oneofValue.value)
							};
							break;
						}
					}
					message[name] = { case: void 0 };
					break;
				}
			}
		}
		return message;
	};
}
/**
* Classify every member once, so that creating a message is a walk over a
* compact list instead of a walk over the descriptor.
*/
function compileInitMessage(desc) {
	var _a, _b;
	const properties = [];
	const prototype = {};
	const usePrototype = needsPrototypeChain(desc);
	for (const member of desc.members) {
		const name = member.localName;
		if (member.kind == "oneof") {
			properties.push({
				name,
				kind: INIT_ONEOF,
				constant: void 0,
				convert: compileConvertOneof(member)
			});
			continue;
		}
		switch (member.fieldKind) {
			case "message":
				properties.push({
					name,
					kind: INIT_SINGULAR,
					constant: void 0,
					convert: compileConvertMessage(member)
				});
				break;
			case "list":
				properties.push({
					name,
					kind: INIT_LIST,
					constant: void 0,
					convert: member.listKind == "message" ? (_a = compileConvertMessage(member)) !== null && _a !== void 0 ? _a : ((value) => value) : member.scalar == ScalarType.BYTES ? toU8Arr : void 0
				});
				break;
			case "map":
				properties.push({
					name,
					kind: INIT_MAP,
					constant: void 0,
					convert: member.mapKind == "message" ? (_b = compileConvertMessage(member)) !== null && _b !== void 0 ? _b : ((value) => value) : member.scalar == ScalarType.BYTES ? toU8Arr : void 0
				});
				break;
			default: {
				const zeroValue = createZeroValue(member);
				properties.push({
					name,
					kind: INIT_SINGULAR,
					constant: member.presence == IMPLICIT$4 ? zeroValue : void 0,
					convert: member.fieldKind == "scalar" && member.scalar == ScalarType.BYTES ? toU8Arr : void 0
				});
				if (usePrototype) prototype[name] = zeroValue;
				break;
			}
		}
	}
	return {
		properties,
		prototype: usePrototype ? prototype : void 0
	};
}
/**
* Compile the conversion of each case of a oneof group, keyed by case name.
*/
function compileConvertOneof(oneof) {
	const converters = /* @__PURE__ */ new Map();
	for (const field of oneof.fields) {
		let convert;
		if (field.fieldKind == "message") convert = compileConvertMessage(field);
		else if (field.fieldKind == "scalar" && field.scalar == ScalarType.BYTES) convert = toU8Arr;
		converters.set(field.localName, convert !== null && convert !== void 0 ? convert : ((value) => value));
	}
	return converters;
}
/**
* Compile the conversion of an init value for a message field, a message
* list item, or a message map value. Returns undefined if values are used
* as-is.
*/
function compileConvertMessage(field) {
	if (field.fieldKind == "message" && !field.oneof && isWrapperDesc(field.message)) return field.message.fields[0].scalar == ScalarType.BYTES ? toU8Arr : void 0;
	if (field.message.typeName == "google.protobuf.Struct" && field.parent.typeName !== "google.protobuf.Value") return;
	const messageDesc = field.message;
	let compiled;
	return (value) => {
		if (!isObject(value) || isMessage(value, messageDesc)) return value;
		compiled !== null && compiled !== void 0 || (compiled = compiledCreate(messageDesc));
		return compiled(value);
	};
}
function toU8Arr(value) {
	return Array.isArray(value) ? new Uint8Array(value) : value;
}
/**
* Do we need the prototype chain to track field presence?
*/
function needsPrototypeChain(desc) {
	switch (desc.file.edition) {
		case EDITION_PROTO3$1: return false;
		case EDITION_PROTO2$1: return true;
		default: return desc.fields.some((f) => f.presence != IMPLICIT$4 && f.fieldKind != "message" && !f.oneof);
	}
}
/**
* Returns the zero value for a scalar or enum field. Scalar and enum fields
* can have default values.
*/
function createZeroValue(field) {
	const defaultValue = field.getDefaultValue();
	if (defaultValue !== void 0) return field.fieldKind == "scalar" && field.longAsString ? defaultValue.toString() : defaultValue;
	return field.fieldKind == "scalar" ? scalarZeroValue(field.scalar, field.longAsString) : field.enum.values[0].number;
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/error.js
const errorNames = [
	"FieldValueInvalidError",
	"FieldListRangeError",
	"ForeignFieldError"
];
var FieldError = class extends Error {
	constructor(fieldOrOneof, message, name = "FieldValueInvalidError") {
		super(message);
		this.name = name;
		this.field = () => fieldOrOneof;
	}
};
function isFieldError(arg) {
	return arg instanceof Error && errorNames.includes(arg.name) && "field" in arg && typeof arg.field == "function";
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/wire/text-encoding.js
let te;
/**
* Protobuf-ES requires the Text Encoding API to convert UTF-8 from and to
* binary. This WHATWG API is widely available, but it is not part of the
* ECMAScript standard. This function used to be the way to supply an
* implementation on runtimes that do not provide one.
*
* It only configures the copy of the library it is imported from. To provide
* the encoding API for every copy of Protobuf-ES in an isolate, install
* `TextEncoder` (with the methods `encode` and optionally `encodeInto`) and
* `TextDecoder` (with the `fatal: true` constructor argument and the `decode`
* method) on `globalThis`.
*
* Providing `encodeUtf8Into` is optional for backwards compatibility. If it
* is omitted, we emulate it with a wrapper that calls `encodeUtf8`.
*
* Note that the Text Encoding API does not provide a way to validate UTF-8.
* Our implementation uses String.prototype.isWellFormed, and falls back
* to use encodeURIComponent().
*
* @deprecated Install `TextEncoder` and `TextDecoder` on `globalThis` instead.
*/
function configureTextEncoding(textEncoding) {
	var _a;
	te = Object.assign(Object.assign({}, textEncoding), { encodeUtf8Into: (_a = textEncoding.encodeUtf8Into) !== null && _a !== void 0 ? _a : emulateEncodeInto(textEncoding.encodeUtf8.bind(textEncoding)) });
}
function getTextEncoding() {
	if (!te) {
		const globals = globalThis;
		if (!globals.TextEncoder || !globals.TextDecoder) throw new Error("encoding API missing: install TextEncoder and TextDecoder on globalThis");
		const textEncoder = new globals.TextEncoder();
		const textDecoder = new globals.TextDecoder();
		let textDecoderStrict;
		const config = {
			encodeUtf8(text) {
				return textEncoder.encode(text);
			},
			decodeUtf8(bytes, strict) {
				if (strict) {
					if (!textDecoderStrict) textDecoderStrict = new globals.TextDecoder("utf-8", { fatal: true });
					return textDecoderStrict.decode(bytes);
				}
				return textDecoder.decode(bytes);
			},
			checkUtf8(text) {
				try {
					return true;
				} catch (_) {
					return false;
				}
			}
		};
		if (textEncoder.encodeInto) config.encodeUtf8Into = textEncoder.encodeInto.bind(textEncoder);
		const nativeStringIsWellFormed = String.prototype.isWellFormed;
		if (nativeStringIsWellFormed) config.checkUtf8 = (text) => {
			return nativeStringIsWellFormed.call(text);
		};
		configureTextEncoding(config);
	}
	return te;
}
/**
* Simplistic polyfill for encodeUtf8Into.
*
* @private
*/
function emulateEncodeInto(encodeUtf8) {
	return (text, dest) => {
		const bytes = encodeUtf8(text);
		dest.set(bytes);
		return { written: bytes.byteLength };
	};
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/wire/binary-encoding.js
/**
* Protobuf binary format wire types.
*
* A wire type provides just enough information to find the length of the
* following value.
*
* See https://developers.google.com/protocol-buffers/docs/encoding#structure
*/
var WireType;
(function(WireType) {
	/**
	* Used for int32, int64, uint32, uint64, sint32, sint64, bool, enum
	*/
	WireType[WireType["Varint"] = 0] = "Varint";
	/**
	* Used for fixed64, sfixed64, double.
	* Always 8 bytes with little-endian byte order.
	*/
	WireType[WireType["Bit64"] = 1] = "Bit64";
	/**
	* Used for string, bytes, embedded messages, packed repeated fields
	*
	* Only repeated numeric types (types which use the varint, 32-bit,
	* or 64-bit wire types) can be packed. In proto3, such fields are
	* packed by default.
	*/
	WireType[WireType["LengthDelimited"] = 2] = "LengthDelimited";
	/**
	* Start of a tag-delimited aggregate, such as a proto2 group, or a message
	* in editions with message_encoding = DELIMITED.
	*/
	WireType[WireType["StartGroup"] = 3] = "StartGroup";
	/**
	* End of a tag-delimited aggregate.
	*/
	WireType[WireType["EndGroup"] = 4] = "EndGroup";
	/**
	* Used for fixed32, sfixed32, float.
	* Always 4 bytes with little-endian byte order.
	*/
	WireType[WireType["Bit32"] = 5] = "Bit32";
})(WireType || (WireType = {}));
var BinaryWriter = class {
	constructor(encodeUtf8) {
		/**
		* Previous fork positions (the write position at the time
		* `fork()` was called).
		*/
		this.stackPos = [];
		this.encodeUtf8Into = encodeUtf8 ? emulateEncodeInto(encodeUtf8) : getTextEncoding().encodeUtf8Into;
		this.buffer = EMPTY_BUFFER;
		this.viewCache = EMPTY_VIEW;
		this.pos = 0;
	}
	ensureCapacity(size) {
		const required = this.pos + size;
		if (required > this.buffer.length) {
			let newLen = this.buffer.length || INITIAL_SIZE;
			while (newLen < required) newLen *= 2;
			const newBuf = new Uint8Array(newLen);
			if (this.pos > 0) newBuf.set(this.buffer);
			this.buffer = newBuf;
		}
	}
	/**
	* The DataView over `buffer`, rebuilt only if the buffer has grown since it
	* was last used.
	*/
	view() {
		const bytes = this.buffer;
		const view = this.viewCache;
		if (view.byteLength === bytes.byteLength) return view;
		const newView = new DataView(bytes.buffer);
		this.viewCache = newView;
		return newView;
	}
	/**
	* Return all bytes written and reset this writer.
	*/
	finish() {
		const result = this.buffer.slice(0, this.pos);
		this.pos = 0;
		this.stackPos = [];
		return result;
	}
	/**
	* Start a new fork for length-delimited data like a message
	* or a packed repeated field.
	*
	* Must be joined later with `join()`.
	*/
	fork() {
		this.stackPos.push(this.pos);
		this.ensureCapacity(DEFAULT_LEN_PREFIX_SIZE);
		this.buffer[this.pos++] = 0;
		return this;
	}
	/**
	* Join the last fork. Write its length and bytes, then
	* return to the previous state.
	*/
	join() {
		const forkPos = this.stackPos.pop();
		if (forkPos === void 0) throw new Error("invalid state, fork stack empty");
		const len = this.pos - forkPos - DEFAULT_LEN_PREFIX_SIZE;
		const lenPrefixSize = varint32Size(len);
		if (lenPrefixSize > DEFAULT_LEN_PREFIX_SIZE) {
			this.ensureCapacity(lenPrefixSize - DEFAULT_LEN_PREFIX_SIZE);
			this.buffer.copyWithin(forkPos + lenPrefixSize, forkPos + DEFAULT_LEN_PREFIX_SIZE, this.pos);
		}
		this.pos = forkPos;
		this.uint32(len);
		this.pos += len;
		return this;
	}
	/**
	* Writes a tag (field number and wire type).
	*
	* Equivalent to `uint32( (fieldNo << 3 | type) >>> 0 )`.
	*
	* Generated code should compute the tag ahead of time and call `uint32()`.
	*/
	tag(fieldNo, type) {
		return this.uint32((fieldNo << 3 | type) >>> 0);
	}
	/**
	* Write a chunk of raw bytes.
	*/
	raw(chunk) {
		this.ensureCapacity(chunk.length);
		this.buffer.set(chunk, this.pos);
		this.pos += chunk.length;
		return this;
	}
	/**
	* Write a `uint32` value, an unsigned 32 bit varint.
	*/
	uint32(value) {
		assertUInt32(value);
		this.ensureCapacity(5);
		if (value < 128) {
			this.buffer[this.pos++] = value;
			return this;
		}
		while (value > 127) {
			this.buffer[this.pos++] = value & 127 | 128;
			value >>>= 7;
		}
		this.buffer[this.pos++] = value;
		return this;
	}
	/**
	* Write a `int32` value, a signed 32 bit varint.
	*/
	int32(value) {
		assertInt32(value);
		if (value >= 0) return this.uint32(value);
		this.ensureCapacity(10);
		for (let i = 0; i < 9; i++) {
			this.buffer[this.pos++] = value & 127 | 128;
			value >>= 7;
		}
		this.buffer[this.pos++] = 1;
		return this;
	}
	/**
	* Write a `bool` value, a varint.
	*/
	bool(value) {
		this.ensureCapacity(1);
		this.buffer[this.pos++] = value ? 1 : 0;
		return this;
	}
	/**
	* Write a `bytes` value, length-delimited arbitrary data.
	*/
	bytes(value) {
		this.uint32(value.byteLength);
		return this.raw(value);
	}
	/**
	* Write a `string` value, length-delimited data converted to UTF-8 text.
	*/
	string(value) {
		if (typeof value !== "string") value = String(value);
		const len = value.length;
		if (len <= ASCII_MAX_LENGTH) {
			this.ensureCapacity(len + 1);
			const ascii = this.buffer;
			let pos = this.pos;
			ascii[pos++] = len;
			let i = 0;
			for (; i < len; i++) {
				const code = value.charCodeAt(i);
				if (code > 127) break;
				ascii[pos++] = code;
			}
			if (i == len) {
				this.pos = pos;
				return this;
			}
		}
		this.ensureCapacity(len * 3 + 5);
		const lenPrefixSizeGuess = varint32Size(len);
		const buf = this.buffer;
		const start = this.pos;
		const { written } = this.encodeUtf8Into(value, buf.subarray(start + lenPrefixSizeGuess));
		const lenPrefixSize = varint32Size(written);
		if (lenPrefixSize != lenPrefixSizeGuess) buf.copyWithin(start + lenPrefixSize, start + lenPrefixSizeGuess, start + lenPrefixSizeGuess + written);
		this.uint32(written);
		this.pos += written;
		return this;
	}
	/**
	* Write a `float` value, 32-bit floating point number.
	*/
	float(value) {
		assertFloat32(value);
		this.ensureCapacity(4);
		this.view().setFloat32(this.pos, value, true);
		this.pos += 4;
		return this;
	}
	/**
	* Write a `double` value, a 64-bit floating point number.
	*/
	double(value) {
		this.ensureCapacity(8);
		this.view().setFloat64(this.pos, value, true);
		this.pos += 8;
		return this;
	}
	/**
	* Write a `fixed32` value, an unsigned, fixed-length 32-bit integer.
	*/
	fixed32(value) {
		assertUInt32(value);
		this.ensureCapacity(4);
		this.view().setUint32(this.pos, value, true);
		this.pos += 4;
		return this;
	}
	/**
	* Write a `sfixed32` value, a signed, fixed-length 32-bit integer.
	*/
	sfixed32(value) {
		assertInt32(value);
		this.ensureCapacity(4);
		this.view().setInt32(this.pos, value, true);
		this.pos += 4;
		return this;
	}
	/**
	* Write a `sint32` value, a signed, zigzag-encoded 32-bit varint.
	*/
	sint32(value) {
		assertInt32(value);
		return this.uint32((value << 1 ^ value >> 31) >>> 0);
	}
	/**
	* Write a `sfixed64` value, a signed, fixed-length 64-bit integer.
	*/
	sfixed64(value) {
		const tc = protoInt64.enc(value);
		this.ensureCapacity(8);
		const view = this.view();
		view.setInt32(this.pos, tc.lo, true);
		view.setInt32(this.pos + 4, tc.hi, true);
		this.pos += 8;
		return this;
	}
	/**
	* Write a `fixed64` value, an unsigned, fixed-length 64 bit integer.
	*/
	fixed64(value) {
		const tc = protoInt64.uEnc(value);
		this.ensureCapacity(8);
		const view = this.view();
		view.setInt32(this.pos, tc.lo, true);
		view.setInt32(this.pos + 4, tc.hi, true);
		this.pos += 8;
		return this;
	}
	/**
	* Write a `int64` value, a signed 64-bit varint.
	*/
	int64(value) {
		const tc = protoInt64.enc(value);
		return this.writeVarint64(tc.lo, tc.hi);
	}
	/**
	* Write a `sint64` value, a signed, zig-zag-encoded 64-bit varint.
	*/
	sint64(value) {
		const tc = protoInt64.enc(value), sign = tc.hi >> 31, lo = tc.lo << 1 ^ sign, hi = (tc.hi << 1 | tc.lo >>> 31) ^ sign;
		return this.writeVarint64(lo, hi);
	}
	/**
	* Write a `uint64` value, an unsigned 64-bit varint.
	*/
	uint64(value) {
		const tc = protoInt64.uEnc(value);
		return this.writeVarint64(tc.lo, tc.hi);
	}
	/**
	* Write a 64-bit varint directly into the buffer. Accepts the value as
	* split low/high 32-bit words.
	*
	* Ported from varint64write() to avoid the intermediate number[] buffer.
	* See https://github.com/protocolbuffers/protobuf/blob/8a71927d74a4ce34efe2d8769fda198f52d20d12/js/experimental/runtime/kernel/writer.js#L344
	*/
	writeVarint64(lo, hi) {
		this.ensureCapacity(10);
		const buf = this.buffer;
		let pos = this.pos;
		for (let i = 0; i < 28; i = i + 7) {
			const shift = lo >>> i;
			const hasNext = !(shift >>> 7 == 0 && hi == 0);
			buf[pos++] = (hasNext ? shift | 128 : shift) & 255;
			if (!hasNext) {
				this.pos = pos;
				return this;
			}
		}
		const splitBits = lo >>> 28 & 15 | (hi & 7) << 4;
		const hasMoreBits = !(hi >> 3 == 0);
		buf[pos++] = (hasMoreBits ? splitBits | 128 : splitBits) & 255;
		if (!hasMoreBits) {
			this.pos = pos;
			return this;
		}
		for (let i = 3; i < 31; i = i + 7) {
			const shift = hi >>> i;
			const hasNext = !(shift >>> 7 == 0);
			buf[pos++] = (hasNext ? shift | 128 : shift) & 255;
			if (!hasNext) {
				this.pos = pos;
				return this;
			}
		}
		buf[pos++] = hi >>> 31 & 1;
		this.pos = pos;
		return this;
	}
};
/**
* Capacity of the buffer allocated by the first write..
*/
const INITIAL_SIZE = 128;
/**
* Bytes `fork()` reserves for the length prefix, betting that the payload will
* be under 128 bytes. `join()` fills them in, and widens them if the bet was
* wrong.
*/
const DEFAULT_LEN_PREFIX_SIZE = 1;
/**
* Shared empty buffer used as the initial value before the first write.
* Avoids allocating and zeroing `INITIAL_SIZE` bytes per BinaryWriter when a
* writer is only used for a tiny message (or not used at all).
*/
const EMPTY_BUFFER = /* @__PURE__ */ new Uint8Array(0);
/**
* Shared empty view, paired with `EMPTY_BUFFER`. Never written to: any
* fixed-width write first grows the buffer, which replaces this view.
*/
const EMPTY_VIEW = new DataView(EMPTY_BUFFER.buffer);
/**
* Longest string on the ASCII fast paths. Must stay below 0x80, so
* that the writer's length prefix always fits a single varint byte.
*/
const ASCII_MAX_LENGTH = 32;
/**
* Number of bytes needed to encode `value` as an unsigned 32-bit varint.
*/
function varint32Size(value) {
	if (value < 128) return 1;
	if (value < 16384) return 2;
	if (value < 2097152) return 3;
	if (value < 268435456) return 4;
	return 5;
}
var BinaryReader = class {
	constructor(buf, decodeUtf8 = getTextEncoding().decodeUtf8) {
		this.decodeUtf8 = decodeUtf8;
		this.varint64Lo = 0;
		this.varint64Hi = 0;
		this.varint64 = varint64read;
		/**
		* Read a `uint32` field, an unsigned 32 bit varint.
		*/
		this.uint32 = varint32read;
		this.buf = buf;
		this.len = buf.length;
		this.pos = 0;
		this.view = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
	}
	/**
	* Reads a tag - field number and wire type. Tags are uint32 varints; values
	* that do not fit in uint32 are rejected.
	*/
	tag() {
		const start = this.pos;
		const tag = this.uint32();
		const bytesRead = this.pos - start;
		if (bytesRead > 5 || bytesRead == 5 && this.buf[this.pos - 1] > 15) throw new Error("illegal tag: varint overflows uint32");
		const fieldNo = tag >>> 3;
		const wireType = tag & 7;
		if (fieldNo <= 0 || wireType > 5) throw new Error("illegal tag: field no " + fieldNo + " wire type " + wireType);
		return [fieldNo, wireType];
	}
	/**
	* Skip one element and return the skipped data.
	*
	* When skipping StartGroup, provide the tags field number to check for
	* matching field number in the EndGroup tag. Recursion into nested groups
	* is guarded by the `recursionLimit` argument: When the limit is reached,
	* this method throws.
	*/
	skip(wireType, fieldNo, recursionLimit = 100) {
		let start = this.pos;
		switch (wireType) {
			case WireType.Varint:
				while (this.buf[this.pos++] & 128);
				break;
			case WireType.Bit64: this.pos += 4;
			case WireType.Bit32:
				this.pos += 4;
				break;
			case WireType.LengthDelimited:
				let len = this.uint32();
				this.pos += len;
				break;
			case WireType.StartGroup:
				if (recursionLimit <= 0) throw new Error("maximum recursion depth reached");
				for (;;) {
					const [fn, wt] = this.tag();
					if (wt === WireType.EndGroup) {
						if (fieldNo !== void 0 && fn !== fieldNo) throw new Error("invalid end group tag");
						break;
					}
					this.skip(wt, fn, recursionLimit - 1);
				}
				break;
			default: throw new Error("cant skip wire type " + wireType);
		}
		this.assertBounds();
		return this.buf.subarray(start, this.pos);
	}
	/**
	* Throws error if position in byte array is out of range.
	*/
	assertBounds() {
		if (this.pos > this.len) throw new RangeError("premature EOF");
	}
	/**
	* Read a `int32` field, a signed 32 bit varint.
	*/
	int32() {
		return this.uint32() | 0;
	}
	/**
	* Read a `sint32` field, a signed, zigzag-encoded 32-bit varint.
	*/
	sint32() {
		let zze = this.uint32();
		return zze >>> 1 ^ -(zze & 1);
	}
	/**
	* Read a `int64` field, a signed 64-bit varint.
	*/
	int64() {
		this.varint64();
		return protoInt64.dec(this.varint64Lo, this.varint64Hi);
	}
	/**
	* Read a `uint64` field, an unsigned 64-bit varint.
	*/
	uint64() {
		this.varint64();
		return protoInt64.uDec(this.varint64Lo, this.varint64Hi);
	}
	/**
	* Read a `sint64` field, a signed, zig-zag-encoded 64-bit varint.
	*/
	sint64() {
		this.varint64();
		let lo = this.varint64Lo;
		let hi = this.varint64Hi;
		let s = -(lo & 1);
		lo = (lo >>> 1 | (hi & 1) << 31) ^ s;
		hi = hi >>> 1 ^ s;
		return protoInt64.dec(lo, hi);
	}
	/**
	* Read a `bool` field, a variant.
	*/
	bool() {
		const b = this.buf[this.pos];
		if (b < 128) {
			this.pos++;
			return b !== 0;
		}
		this.varint64();
		return this.varint64Lo !== 0 || this.varint64Hi !== 0;
	}
	/**
	* Read a `fixed32` field, an unsigned, fixed-length 32-bit integer.
	*/
	fixed32() {
		return this.view.getUint32((this.pos += 4) - 4, true);
	}
	/**
	* Read a `sfixed32` field, a signed, fixed-length 32-bit integer.
	*/
	sfixed32() {
		return this.view.getInt32((this.pos += 4) - 4, true);
	}
	/**
	* Read a `fixed64` field, an unsigned, fixed-length 64 bit integer.
	*/
	fixed64() {
		return protoInt64.uDec(this.sfixed32(), this.sfixed32());
	}
	/**
	* Read a `fixed64` field, a signed, fixed-length 64-bit integer.
	*/
	sfixed64() {
		return protoInt64.dec(this.sfixed32(), this.sfixed32());
	}
	/**
	* Read a `float` field, 32-bit floating point number.
	*/
	float() {
		return this.view.getFloat32((this.pos += 4) - 4, true);
	}
	/**
	* Read a `double` field, a 64-bit floating point number.
	*/
	double() {
		return this.view.getFloat64((this.pos += 8) - 8, true);
	}
	/**
	* Read a `bytes` field, length-delimited arbitrary data.
	*/
	bytes() {
		let len = this.uint32(), start = this.pos;
		this.pos += len;
		this.assertBounds();
		return this.buf.subarray(start, start + len);
	}
	/**
	* Read a `string` field, length-delimited data converted to UTF-8 text. If
	* `strict` is true, throw on invalid UTF-8 instead of substituting U+FFFD.
	*/
	string(strict) {
		const bytes = this.bytes();
		const len = bytes.length;
		if (len <= ASCII_MAX_LENGTH) {
			const codes = new Array(len);
			for (let i = 0; i < len; i++) {
				const byte = bytes[i];
				if (byte > 127) return this.decodeUtf8(bytes, strict);
				codes[i] = byte;
			}
			return String.fromCharCode.apply(String, codes);
		}
		return this.decodeUtf8(bytes, strict);
	}
};
/**
* Assert a valid signed protobuf 32-bit integer as a number or string.
*/
function assertInt32(arg) {
	if (typeof arg == "string") arg = Number(arg);
	else if (typeof arg != "number") throw new Error("invalid int32: " + typeof arg);
	if (!Number.isInteger(arg) || arg > 2147483647 || arg < -2147483648) throw new Error("invalid int32: " + arg);
}
/**
* Assert a valid unsigned protobuf 32-bit integer as a number or string.
*/
function assertUInt32(arg) {
	if (typeof arg == "string") arg = Number(arg);
	else if (typeof arg != "number") throw new Error("invalid uint32: " + typeof arg);
	if (!Number.isInteger(arg) || arg > 4294967295 || arg < 0) throw new Error("invalid uint32: " + arg);
}
/**
* Assert a valid protobuf float32 value as a number or string.
*/
function assertFloat32(arg) {
	if (typeof arg == "string") {
		const o = arg;
		arg = Number(arg);
		if (Number.isNaN(arg) && o !== "NaN") throw new Error("invalid float32: " + o);
	} else if (typeof arg != "number") throw new Error("invalid float32: " + typeof arg);
	if (!Number.isFinite(Math.fround(arg)) && Number.isFinite(arg)) throw new Error("invalid float32: " + arg);
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/reflect-check.js
/**
* Check whether the given field value is valid for the reflect API.
*/
function checkField(field, value) {
	const check = field.fieldKind == "list" ? isReflectList(value, field) : field.fieldKind == "map" ? isReflectMap(value, field) : checkSingular(field, value);
	if (check === true) return;
	let reason;
	switch (field.fieldKind) {
		case "list":
			reason = `expected ${formatReflectList(field)}, got ${formatVal(value)}`;
			break;
		case "map":
			reason = `expected ${formatReflectMap(field)}, got ${formatVal(value)}`;
			break;
		default: reason = reasonSingular(field, value, check);
	}
	return new FieldError(field, reason);
}
/**
* Check whether the given list item is valid for the reflect API.
*/
function checkListItem(field, index, value) {
	const check = checkSingular(field, value);
	if (check !== true) return new FieldError(field, `list item #${index + 1}: ${reasonSingular(field, value, check)}`);
}
/**
* Check whether the given map key and value are valid for the reflect API.
*/
function checkMapEntry(field, key, value) {
	const checkKey = checkScalarValue(field.mapKey)(key);
	if (checkKey !== true) return new FieldError(field, `invalid map key: ${reasonSingular({ scalar: field.mapKey }, key, checkKey)}`);
	const checkVal = checkSingular(field, value);
	if (checkVal !== true) return new FieldError(field, `map entry ${formatVal(key)}: ${reasonSingular(field, value, checkVal)}`);
}
function checkSingular(field, value) {
	if (field.scalar !== void 0) return checkScalarValue(field.scalar)(value);
	if (field.enum !== void 0) {
		if (field.enum.open) return checkScalarValue(ScalarType.INT32)(value);
		return field.enum.values.some((v) => v.number === value);
	}
	return isReflectMessage(value, field.message);
}
/**
* Return the check for values of the given scalar type.
*
* @private
*/
function checkScalarValue(scalar) {
	switch (scalar) {
		case ScalarType.DOUBLE: return (value) => typeof value == "number";
		case ScalarType.FLOAT: return (value) => {
			if (typeof value != "number") return false;
			if (!Number.isFinite(Math.fround(value)) && Number.isFinite(value)) return `${value.toFixed()} out of range`;
			return true;
		};
		case ScalarType.INT32:
		case ScalarType.SFIXED32:
		case ScalarType.SINT32: return (value) => {
			if (typeof value !== "number" || !Number.isInteger(value)) return false;
			if (value > 2147483647 || value < -2147483648) return `${value.toFixed()} out of range`;
			return true;
		};
		case ScalarType.FIXED32:
		case ScalarType.UINT32: return (value) => {
			if (typeof value !== "number" || !Number.isInteger(value)) return false;
			if (value > 4294967295 || value < 0) return `${value.toFixed()} out of range`;
			return true;
		};
		case ScalarType.BOOL: return (value) => typeof value == "boolean";
		case ScalarType.STRING: return (value) => {
			if (typeof value != "string") return false;
			return getTextEncoding().checkUtf8(value) || "invalid UTF8";
		};
		case ScalarType.BYTES: return (value) => value instanceof Uint8Array;
		case ScalarType.INT64:
		case ScalarType.SFIXED64:
		case ScalarType.SINT64: return (value) => {
			if (typeof value == "bigint" || typeof value == "number" || typeof value == "string" && value.length > 0) try {
				protoInt64.parse(value);
				return true;
			} catch (_) {
				return `${value} out of range`;
			}
			return false;
		};
		case ScalarType.FIXED64:
		case ScalarType.UINT64: return (value) => {
			if (typeof value == "bigint" || typeof value == "number" || typeof value == "string" && value.length > 0) try {
				protoInt64.uParse(value);
				return true;
			} catch (_) {
				return `${value} out of range`;
			}
			return false;
		};
	}
}
/**
* Format the reason why a value is invalid for a singular field.
*
* @private
*/
function reasonSingular(field, val, details) {
	details = typeof details == "string" ? `: ${details}` : `, got ${formatVal(val)}`;
	if (field.scalar !== void 0) return `expected ${scalarTypeDescription(field.scalar)}` + details;
	if (field.enum !== void 0) return `expected ${field.enum.toString()}` + details;
	return `expected ${formatReflectMessage(field.message)}` + details;
}
function formatVal(val) {
	switch (typeof val) {
		case "object":
			if (val === null) return "null";
			if (val instanceof Uint8Array) return `Uint8Array(${val.length})`;
			if (Array.isArray(val)) return `Array(${val.length})`;
			if (isReflectList(val)) return formatReflectList(val.field());
			if (isReflectMap(val)) return formatReflectMap(val.field());
			if (isReflectMessage(val)) return formatReflectMessage(val.desc);
			if (isMessage(val)) return `message ${val.$typeName}`;
			return "object";
		case "string": return val.length > 30 ? "string" : `"${val.split("\"").join("\\\"")}"`;
		case "boolean": return String(val);
		case "number": return String(val);
		case "bigint": return String(val) + "n";
		default: return typeof val;
	}
}
function formatReflectMessage(desc) {
	return `ReflectMessage (${desc.typeName})`;
}
function formatReflectList(field) {
	switch (field.listKind) {
		case "message": return `ReflectList (${field.message.toString()})`;
		case "enum": return `ReflectList (${field.enum.toString()})`;
		case "scalar": return `ReflectList (${ScalarType[field.scalar]})`;
	}
}
function formatReflectMap(field) {
	switch (field.mapKind) {
		case "message": return `ReflectMap (${ScalarType[field.mapKey]}, ${field.message.toString()})`;
		case "enum": return `ReflectMap (${ScalarType[field.mapKey]}, ${field.enum.toString()})`;
		case "scalar": return `ReflectMap (${ScalarType[field.mapKey]}, ${ScalarType[field.scalar]})`;
	}
}
function scalarTypeDescription(scalar) {
	switch (scalar) {
		case ScalarType.STRING: return "string";
		case ScalarType.BOOL: return "boolean";
		case ScalarType.INT64:
		case ScalarType.SINT64:
		case ScalarType.SFIXED64: return "bigint (int64)";
		case ScalarType.UINT64:
		case ScalarType.FIXED64: return "bigint (uint64)";
		case ScalarType.BYTES: return "Uint8Array";
		case ScalarType.DOUBLE: return "number (float64)";
		case ScalarType.FLOAT: return "number (float32)";
		case ScalarType.FIXED32:
		case ScalarType.UINT32: return "number (uint32)";
		case ScalarType.INT32:
		case ScalarType.SFIXED32:
		case ScalarType.SINT32: return "number (int32)";
	}
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/message.js
const NULL_VALUE = 0;
/**
* Return the conversions between the local representation of the field
* value and the message it represents.
*
* @private
*/
function localMessageMapper(field) {
	if (usesJsonRepresentation(field)) return {
		toMessage: (local) => wktStructToReflect(local),
		toLocal: (message) => wktStructToLocal(message)
	};
	if (field.fieldKind == "message" && !field.oneof && isWrapperDesc(field.message)) {
		const wrapperDesc = field.message;
		const valueLocalName = wrapperDesc.fields[0].localName;
		return {
			toMessage: (local) => {
				const message = create$1(wrapperDesc);
				if (local !== void 0) message[valueLocalName] = local;
				return message;
			},
			toLocal: (message) => message[valueLocalName]
		};
	}
	const childDesc = field.message;
	return {
		toMessage: (local) => local === void 0 ? create$1(childDesc) : local,
		toLocal: (message) => message
	};
}
/**
* Returns true if values of this field are stored as JsonValue instead of
* a message: google.protobuf.Struct is represented with JsonObject when
* used in a field, except when used in google.protobuf.Value.
*/
function usesJsonRepresentation(field) {
	return field.message.typeName == "google.protobuf.Struct" && field.parent.typeName != "google.protobuf.Value";
}
/**
* Convert the JsonValue representation of a google.protobuf.Struct to the
* message representation.
*
* @private
*/
function wktStructToReflect(json) {
	const struct = {
		$typeName: "google.protobuf.Struct",
		fields: {}
	};
	if (isObject(json)) for (const k of Object.keys(json)) setOwn(struct.fields, k, wktValueToReflect(json[k]));
	return struct;
}
/**
* Convert a google.protobuf.Struct message to its JsonValue representation.
*
* @private
*/
function wktStructToLocal(val) {
	const json = {};
	for (const k of Object.keys(val.fields)) setOwn(json, k, wktValueToLocal(val.fields[k]));
	return json;
}
function wktValueToLocal(val) {
	switch (val.kind.case) {
		case "structValue": return wktStructToLocal(val.kind.value);
		case "listValue": return val.kind.value.values.map(wktValueToLocal);
		case "nullValue":
		case void 0: return null;
		default: return val.kind.value;
	}
}
function wktValueToReflect(json) {
	const value = {
		$typeName: "google.protobuf.Value",
		kind: { case: void 0 }
	};
	switch (typeof json) {
		case "number":
			value.kind = {
				case: "numberValue",
				value: json
			};
			break;
		case "string":
			value.kind = {
				case: "stringValue",
				value: json
			};
			break;
		case "boolean":
			value.kind = {
				case: "boolValue",
				value: json
			};
			break;
		case "object": if (json === null) value.kind = {
			case: "nullValue",
			value: NULL_VALUE
		};
		else if (Array.isArray(json)) {
			const listValue = {
				$typeName: "google.protobuf.ListValue",
				values: []
			};
			if (Array.isArray(json)) for (const e of json) listValue.values.push(wktValueToReflect(e));
			value.kind = {
				case: "listValue",
				value: listValue
			};
		} else value.kind = {
			case: "structValue",
			value: wktStructToReflect(json)
		};
	}
	return value;
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/reflect.js
/**
* Create a ReflectMessage.
*/
function reflect(messageDesc, message, check = true) {
	return new ReflectMessageImpl(messageDesc, message, check);
}
const messageSortedFields = /* @__PURE__ */ new WeakMap();
var ReflectMessageImpl = class {
	get sortedFields() {
		const cached = messageSortedFields.get(this.desc);
		if (cached) return cached;
		const sortedFields = this.desc.fields.concat().sort((a, b) => a.number - b.number);
		messageSortedFields.set(this.desc, sortedFields);
		return sortedFields;
	}
	constructor(messageDesc, message, check = true) {
		this.lists = /* @__PURE__ */ new Map();
		this.maps = /* @__PURE__ */ new Map();
		this.check = check;
		this.desc = messageDesc;
		this.message = this[unsafeLocal] = message !== null && message !== void 0 ? message : create$1(messageDesc);
		this.fields = messageDesc.fields;
		this.oneofs = messageDesc.oneofs;
		this.members = messageDesc.members;
	}
	findNumber(number) {
		if (!this._fieldsByNumber) this._fieldsByNumber = new Map(this.desc.fields.map((f) => [f.number, f]));
		return this._fieldsByNumber.get(number);
	}
	oneofCase(oneof) {
		assertOwn(this.message, oneof);
		return unsafeOneofCase(this.message, oneof);
	}
	isSet(field) {
		assertOwn(this.message, field);
		return unsafeIsSet(this.message, field);
	}
	clear(field) {
		assertOwn(this.message, field);
		unsafeClear(this.message, field);
	}
	get(field) {
		assertOwn(this.message, field);
		const value = unsafeGet(this.message, field);
		switch (field.fieldKind) {
			case "list":
				let list = this.lists.get(field);
				if (!list || list[unsafeLocal] !== value) this.lists.set(field, list = new ReflectListImpl(field, value, this.check));
				return list;
			case "map":
				let map = this.maps.get(field);
				if (!map || map[unsafeLocal] !== value) this.maps.set(field, map = new ReflectMapImpl(field, value, this.check));
				return map;
			case "message": return messageToReflect(field, value, this.check);
			case "scalar": return value === void 0 ? scalarZeroValue(field.scalar, false) : longToReflect(field, value);
			case "enum": return value !== null && value !== void 0 ? value : field.enum.values[0].number;
		}
	}
	set(field, value) {
		assertOwn(this.message, field);
		if (this.check) {
			const err = checkField(field, value);
			if (err) throw err;
		}
		let local;
		if (field.fieldKind == "message") local = messageToLocal(field, value);
		else if (isReflectMap(value) || isReflectList(value)) local = value[unsafeLocal];
		else local = longToLocal(field, value);
		unsafeSet(this.message, field, local);
	}
	getUnknown() {
		return this.message.$unknown;
	}
	setUnknown(value) {
		this.message.$unknown = value;
	}
};
function assertOwn(owner, member) {
	if (member.parent.typeName !== owner.$typeName) throw new FieldError(member, `cannot use ${member.toString()} with message ${owner.$typeName}`, "ForeignFieldError");
}
var ReflectListImpl = class {
	field() {
		return this._field;
	}
	get size() {
		return this._arr.length;
	}
	constructor(field, unsafeInput, check) {
		this._field = field;
		this._arr = this[unsafeLocal] = unsafeInput;
		this.check = check;
	}
	get(index) {
		const item = this._arr[index];
		return item === void 0 ? void 0 : listItemToReflect(this._field, item, this.check);
	}
	set(index, item) {
		if (index < 0 || index >= this._arr.length) throw new FieldError(this._field, `list item #${index + 1}: out of range`);
		if (this.check) {
			const err = checkListItem(this._field, index, item);
			if (err) throw err;
		}
		this._arr[index] = listItemToLocal(this._field, item);
	}
	add(item) {
		if (this.check) {
			const err = checkListItem(this._field, this._arr.length, item);
			if (err) throw err;
		}
		this._arr.push(listItemToLocal(this._field, item));
	}
	clear() {
		this._arr.splice(0, this._arr.length);
	}
	[Symbol.iterator]() {
		return this.values();
	}
	keys() {
		return this._arr.keys();
	}
	*values() {
		for (const item of this._arr) yield listItemToReflect(this._field, item, this.check);
	}
	*entries() {
		for (let i = 0; i < this._arr.length; i++) yield [i, listItemToReflect(this._field, this._arr[i], this.check)];
	}
};
var ReflectMapImpl = class {
	constructor(field, unsafeInput, check = true) {
		this.obj = this[unsafeLocal] = unsafeInput !== null && unsafeInput !== void 0 ? unsafeInput : {};
		this.check = check;
		this._field = field;
	}
	field() {
		return this._field;
	}
	set(key, value) {
		if (this.check) {
			const err = checkMapEntry(this._field, key, value);
			if (err) throw err;
		}
		setOwn(this.obj, mapKeyToLocal(key), mapValueToLocal(this._field, value));
		return this;
	}
	delete(key) {
		const k = mapKeyToLocal(key);
		const has = Object.prototype.hasOwnProperty.call(this.obj, k);
		if (has) delete this.obj[k];
		return has;
	}
	clear() {
		for (const key of Object.keys(this.obj)) delete this.obj[key];
	}
	get(key) {
		let val = getOwn(this.obj, mapKeyToLocal(key));
		if (val !== void 0) val = mapValueToReflect(this._field, val, this.check);
		return val;
	}
	has(key) {
		return Object.prototype.hasOwnProperty.call(this.obj, mapKeyToLocal(key));
	}
	*keys() {
		for (const objKey of Object.keys(this.obj)) yield mapKeyToReflect(objKey, this._field.mapKey);
	}
	*entries() {
		for (const objEntry of Object.entries(this.obj)) yield [mapKeyToReflect(objEntry[0], this._field.mapKey), mapValueToReflect(this._field, objEntry[1], this.check)];
	}
	[Symbol.iterator]() {
		return this.entries();
	}
	get size() {
		return Object.keys(this.obj).length;
	}
	*values() {
		for (const val of Object.values(this.obj)) yield mapValueToReflect(this._field, val, this.check);
	}
	forEach(callbackfn, thisArg) {
		for (const mapEntry of this.entries()) callbackfn.call(thisArg, mapEntry[1], mapEntry[0], this);
	}
};
function messageToLocal(field, value) {
	if (!isReflectMessage(value)) return value;
	if (isWrapper(value.message) && !field.oneof && field.fieldKind == "message") return value.message.value;
	if (value.desc.typeName == "google.protobuf.Struct" && field.parent.typeName != "google.protobuf.Value") return wktStructToLocal(value.message);
	return value.message;
}
function messageToReflect(field, value, check) {
	if (value !== void 0) {
		if (isWrapperDesc(field.message) && !field.oneof && field.fieldKind == "message") value = {
			$typeName: field.message.typeName,
			value: longToReflect(field.message.fields[0], value)
		};
		else if (field.message.typeName == "google.protobuf.Struct" && field.parent.typeName != "google.protobuf.Value" && isObject(value)) value = wktStructToReflect(value);
	}
	return new ReflectMessageImpl(field.message, value, check);
}
function listItemToLocal(field, value) {
	if (field.listKind == "message") return messageToLocal(field, value);
	return longToLocal(field, value);
}
function listItemToReflect(field, value, check) {
	if (field.listKind == "message") return messageToReflect(field, value, check);
	return longToReflect(field, value);
}
function mapValueToLocal(field, value) {
	if (field.mapKind == "message") return messageToLocal(field, value);
	return longToLocal(field, value);
}
function mapValueToReflect(field, value, check) {
	if (field.mapKind == "message") return messageToReflect(field, value, check);
	return value;
}
function mapKeyToLocal(key) {
	return typeof key == "string" || typeof key == "number" ? key : String(key);
}
/**
* Converts a map key (any scalar value except float, double, or bytes) from its
* representation in a message (string or number, the only possible object key
* types) to the closest possible type in ECMAScript.
*/
function mapKeyToReflect(key, type) {
	switch (type) {
		case ScalarType.STRING: return key;
		case ScalarType.INT32:
		case ScalarType.FIXED32:
		case ScalarType.UINT32:
		case ScalarType.SFIXED32:
		case ScalarType.SINT32: {
			const n = Number.parseInt(key);
			if (Number.isFinite(n)) return n;
			break;
		}
		case ScalarType.BOOL:
			switch (key) {
				case "true": return true;
				case "false": return false;
			}
			break;
		case ScalarType.UINT64:
		case ScalarType.FIXED64:
			try {
				return protoInt64.uParse(key);
			} catch (_a) {}
			break;
		default: try {
			return protoInt64.parse(key);
		} catch (_b) {}
	}
	return key;
}
function longToReflect(field, value) {
	switch (field.scalar) {
		case ScalarType.INT64:
		case ScalarType.SFIXED64:
		case ScalarType.SINT64:
			if ("longAsString" in field && field.longAsString && typeof value == "string") value = protoInt64.parse(value);
			break;
		case ScalarType.FIXED64:
		case ScalarType.UINT64: if ("longAsString" in field && field.longAsString && typeof value == "string") value = protoInt64.uParse(value);
	}
	return value;
}
function longToLocal(field, value) {
	switch (field.scalar) {
		case ScalarType.INT64:
		case ScalarType.SFIXED64:
		case ScalarType.SINT64:
			if ("longAsString" in field && field.longAsString) value = String(value);
			else if (typeof value == "string" || typeof value == "number") value = protoInt64.parse(value);
			break;
		case ScalarType.FIXED64:
		case ScalarType.UINT64: if ("longAsString" in field && field.longAsString) value = String(value);
		else if (typeof value == "string" || typeof value == "number") value = protoInt64.uParse(value);
	}
	return value;
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/wire/base64-encoding.js
const nativeSetFromBase64 = Uint8Array.prototype.setFromBase64;
/**
* Decodes a base64 string to a byte array.
*
* - ignores white-space, including line breaks and tabs
* - allows inner padding (can decode concatenated base64 strings)
* - does not require padding
* - understands base64url encoding:
*   "-" instead of "+",
*   "_" instead of "/",
*   no padding
*/
function base64Decode(base64Str) {
	const len = base64Str.length;
	let size = len - (len + 3 >> 2);
	if ((len & 3) == 0 && base64Str[len - 1] == "=") size -= base64Str[len - 2] == "=" ? 2 : 1;
	const bytes = new Uint8Array(size);
	let written = -1;
	if (nativeSetFromBase64) try {
		const result = nativeSetFromBase64.call(bytes, base64Str);
		if (result.read == len) written = result.written;
	} catch (_a) {}
	if (written < 0) written = setFromBase64(bytes, base64Str);
	return written == size ? bytes : bytes.subarray(0, written);
}
/** Writes into `bytes` from index 0 and returns the number of bytes written. */
function setFromBase64(bytes, base64Str) {
	const table = getDecodeTable();
	let bytePos = 0, groupPos = 0, b, p = 0;
	for (let i = 0; i < base64Str.length; i++) {
		b = table[base64Str.charCodeAt(i)];
		if (b === void 0) switch (base64Str[i]) {
			case "=": groupPos = 0;
			case "\n":
			case "\r":
			case "	":
			case " ": continue;
			default: throw Error("invalid base64 string");
		}
		switch (groupPos) {
			case 0:
				p = b;
				groupPos = 1;
				break;
			case 1:
				bytes[bytePos++] = p << 2 | (b & 48) >> 4;
				p = b;
				groupPos = 2;
				break;
			case 2:
				bytes[bytePos++] = (p & 15) << 4 | (b & 60) >> 2;
				p = b;
				groupPos = 3;
				break;
			case 3:
				bytes[bytePos++] = (p & 3) << 6 | b;
				groupPos = 0;
		}
	}
	if (groupPos == 1) throw Error("invalid base64 string");
	return bytePos;
}
const nativeToBase64 = Uint8Array.prototype.toBase64;
const toBase64OptionsMap = {
	std: {
		alphabet: "base64",
		omitPadding: false
	},
	std_raw: {
		alphabet: "base64",
		omitPadding: true
	},
	url: {
		alphabet: "base64url",
		omitPadding: true
	}
};
/**
* Encode a byte array to a base64 string.
*
* By default, this function uses the standard base64 encoding with padding.
*
* To encode without padding, use encoding = "std_raw".
*
* To encode with the URL encoding, use encoding = "url", which replaces the
* characters +/ by their URL-safe counterparts -_, and omits padding.
*/
function base64Encode(bytes, encoding = "std") {
	if (nativeToBase64) return nativeToBase64.call(bytes, toBase64OptionsMap[encoding]);
	const table = getEncodeTable(encoding);
	const pad = encoding == "std";
	let base64 = "", groupPos = 0, b, p = 0;
	for (let i = 0; i < bytes.length; i++) {
		b = bytes[i];
		switch (groupPos) {
			case 0:
				base64 += table[b >> 2];
				p = (b & 3) << 4;
				groupPos = 1;
				break;
			case 1:
				base64 += table[p | b >> 4];
				p = (b & 15) << 2;
				groupPos = 2;
				break;
			case 2:
				base64 += table[p | b >> 6];
				base64 += table[b & 63];
				groupPos = 0;
		}
	}
	if (groupPos) {
		base64 += table[p];
		if (pad) {
			base64 += "=";
			if (groupPos == 1) base64 += "=";
		}
	}
	return base64;
}
let encodeTableStd;
let encodeTableUrl;
let decodeTable;
function getEncodeTable(encoding) {
	if (!encodeTableStd) {
		encodeTableStd = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/".split("");
		encodeTableUrl = encodeTableStd.slice(0, -2).concat("-", "_");
	}
	return encoding == "url" ? encodeTableUrl : encodeTableStd;
}
function getDecodeTable() {
	if (!decodeTable) {
		decodeTable = [];
		const encodeTable = getEncodeTable("std");
		for (let i = 0; i < encodeTable.length; i++) decodeTable[encodeTable[i].charCodeAt(0)] = i;
		decodeTable["-".charCodeAt(0)] = encodeTable.indexOf("+");
		decodeTable["_".charCodeAt(0)] = encodeTable.indexOf("/");
	}
	return decodeTable;
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/names.js
/**
* Converts snake_case to protoCamelCase according to the convention
* used by protoc to convert a field name to a JSON name.
*
* See https://protobuf.com/docs/language-spec#default-json-names
*
* The function protoSnakeCase provides the reverse.
*/
function protoCamelCase(snakeCase) {
	let capNext = false;
	const b = [];
	for (let i = 0; i < snakeCase.length; i++) {
		let c = snakeCase.charAt(i);
		switch (c) {
			case "_":
				capNext = true;
				break;
			case "0":
			case "1":
			case "2":
			case "3":
			case "4":
			case "5":
			case "6":
			case "7":
			case "8":
			case "9":
				b.push(c);
				capNext = false;
				break;
			default:
				if (capNext) {
					capNext = false;
					c = c.toUpperCase();
				}
				b.push(c);
		}
	}
	return b.join("");
}
/**
* Converts protoCamelCase to snake_case.
*
* This function is the reverse of function protoCamelCase. Note that some names
* are not reversible - for example, "foo__bar" -> "fooBar" -> "foo_bar".
*/
function protoSnakeCase(lowerCamelCase) {
	return lowerCamelCase.replace(/[A-Z]/g, (letter) => "_" + letter.toLowerCase());
}
/**
* Names that cannot be used for object properties because they are reserved
* by built-in JavaScript properties.
*/
const reservedObjectProperties = /* @__PURE__ */ new Set([
	"constructor",
	"toString",
	"toJSON",
	"valueOf"
]);
/**
* Escapes names that are reserved for ECMAScript built-in object properties.
*
* Also see safeIdentifier() from @bufbuild/protoplugin.
*/
function safeObjectProperty(name) {
	return reservedObjectProperties.has(name) ? name + "$" : name;
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/restore-json-names.js
/**
* @private
*/
function restoreJsonNames(message) {
	for (const f of message.field) if (!unsafeIsSetExplicit(f, "jsonName")) f.jsonName = protoCamelCase(f.name);
	message.nestedType.forEach(restoreJsonNames);
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/wire/text-format.js
/**
* Parse an enum value from the Protobuf text format.
*
* @private
*/
function parseTextFormatEnumValue(descEnum, value) {
	const enumValue = descEnum.values.find((v) => v.name === value);
	if (!enumValue) throw new Error(`cannot parse ${descEnum} default value: ${value}`);
	return enumValue.number;
}
/**
* Parse a scalar value from the Protobuf text format.
*
* @private
*/
function parseTextFormatScalarValue(type, value) {
	switch (type) {
		case ScalarType.STRING: return value;
		case ScalarType.BYTES: {
			const u = unescapeBytesDefaultValue(value);
			if (u === false) throw new Error(`cannot parse ${ScalarType[type]} default value: ${value}`);
			return u;
		}
		case ScalarType.INT64:
		case ScalarType.SFIXED64:
		case ScalarType.SINT64: return protoInt64.parse(value);
		case ScalarType.UINT64:
		case ScalarType.FIXED64: return protoInt64.uParse(value);
		case ScalarType.DOUBLE:
		case ScalarType.FLOAT: switch (value) {
			case "inf": return Number.POSITIVE_INFINITY;
			case "-inf": return Number.NEGATIVE_INFINITY;
			case "nan": return NaN;
			default: return parseFloat(value);
		}
		case ScalarType.BOOL: return value === "true";
		case ScalarType.INT32:
		case ScalarType.UINT32:
		case ScalarType.SINT32:
		case ScalarType.FIXED32:
		case ScalarType.SFIXED32: return parseInt(value, 10);
	}
}
/**
* Parses a text-encoded default value (proto2) of a BYTES field.
*/
function unescapeBytesDefaultValue(str) {
	const b = [];
	const input = {
		tail: str,
		c: "",
		next() {
			if (this.tail.length == 0) return false;
			this.c = this.tail[0];
			this.tail = this.tail.substring(1);
			return true;
		},
		take(n) {
			if (this.tail.length >= n) {
				const r = this.tail.substring(0, n);
				this.tail = this.tail.substring(n);
				return r;
			}
			return false;
		}
	};
	while (input.next()) switch (input.c) {
		case "\\":
			if (input.next()) switch (input.c) {
				case "\\":
					b.push(input.c.charCodeAt(0));
					break;
				case "b":
					b.push(8);
					break;
				case "f":
					b.push(12);
					break;
				case "n":
					b.push(10);
					break;
				case "r":
					b.push(13);
					break;
				case "t":
					b.push(9);
					break;
				case "v":
					b.push(11);
					break;
				case "0":
				case "1":
				case "2":
				case "3":
				case "4":
				case "5":
				case "6":
				case "7": {
					const s = input.c;
					const t = input.take(2);
					if (t === false) return false;
					const n = parseInt(s + t, 8);
					if (Number.isNaN(n)) return false;
					b.push(n);
					break;
				}
				case "x": {
					const s = input.c;
					const t = input.take(2);
					if (t === false) return false;
					const n = parseInt(s + t, 16);
					if (Number.isNaN(n)) return false;
					b.push(n);
					break;
				}
				case "u": {
					const s = input.c;
					const t = input.take(4);
					if (t === false) return false;
					const n = parseInt(s + t, 16);
					if (Number.isNaN(n)) return false;
					const chunk = /* @__PURE__ */ new Uint8Array(4);
					new DataView(chunk.buffer).setInt32(0, n, true);
					b.push(chunk[0], chunk[1], chunk[2], chunk[3]);
					break;
				}
				case "U": {
					const s = input.c;
					const t = input.take(8);
					if (t === false) return false;
					const tc = protoInt64.uEnc(s + t);
					const chunk = /* @__PURE__ */ new Uint8Array(8);
					const view = new DataView(chunk.buffer);
					view.setInt32(0, tc.lo, true);
					view.setInt32(4, tc.hi, true);
					b.push(chunk[0], chunk[1], chunk[2], chunk[3], chunk[4], chunk[5], chunk[6], chunk[7]);
					break;
				}
			}
			break;
		default: b.push(input.c.charCodeAt(0));
	}
	return new Uint8Array(b);
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/reflect/nested-types.js
/**
* Iterate over all types - enumerations, extensions, services, messages -
* and enumerations, extensions and messages nested in messages.
*/
function* nestedTypes(desc) {
	switch (desc.kind) {
		case "file":
			for (const message of desc.messages) {
				yield message;
				yield* nestedTypes(message);
			}
			yield* desc.enums;
			yield* desc.services;
			yield* desc.extensions;
			break;
		case "message":
			for (const message of desc.nestedMessages) {
				yield message;
				yield* nestedTypes(message);
			}
			yield* desc.nestedEnums;
			yield* desc.nestedExtensions;
	}
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/registry.js
function createFileRegistry(...args) {
	const registry = createBaseRegistry();
	if (!args.length) return registry;
	if ("$typeName" in args[0] && args[0].$typeName == "google.protobuf.FileDescriptorSet") {
		for (const file of args[0].file) addFile(file, registry);
		return registry;
	}
	if ("$typeName" in args[0]) {
		const input = args[0];
		const resolve = args[1];
		const seen = /* @__PURE__ */ new Set();
		function recurseDeps(file) {
			const deps = [];
			for (const protoFileName of file.dependency) {
				if (registry.getFile(protoFileName) != void 0) continue;
				if (seen.has(protoFileName)) continue;
				const dep = resolve(protoFileName);
				if (!dep) throw new Error(`Unable to resolve ${protoFileName}, imported by ${file.name}`);
				if ("kind" in dep) registry.addFile(dep, false, true);
				else {
					seen.add(dep.name);
					deps.push(dep);
				}
			}
			return deps.concat(...deps.map(recurseDeps));
		}
		for (const file of [input, ...recurseDeps(input)].reverse()) addFile(file, registry);
	} else for (const fileReg of args) for (const file of fileReg.files) registry.addFile(file);
	return registry;
}
/**
* @private
*/
function createBaseRegistry() {
	const types = /* @__PURE__ */ new Map();
	const extendees = /* @__PURE__ */ new Map();
	const files = /* @__PURE__ */ new Map();
	return {
		kind: "registry",
		types,
		extendees,
		[Symbol.iterator]() {
			return types.values();
		},
		get files() {
			return files.values();
		},
		addFile(file, skipTypes, withDeps) {
			files.set(file.proto.name, file);
			if (!skipTypes) for (const type of nestedTypes(file)) this.add(type);
			if (withDeps) for (const f of file.dependencies) this.addFile(f, skipTypes, withDeps);
		},
		add(desc) {
			if (desc.kind == "extension") {
				let numberToExt = extendees.get(desc.extendee.typeName);
				if (!numberToExt) extendees.set(desc.extendee.typeName, numberToExt = /* @__PURE__ */ new Map());
				numberToExt.set(desc.number, desc);
			}
			types.set(desc.typeName, desc);
		},
		get(typeName) {
			return types.get(typeName);
		},
		getFile(fileName) {
			return files.get(fileName);
		},
		getMessage(typeName) {
			const t = types.get(typeName);
			return (t === null || t === void 0 ? void 0 : t.kind) == "message" ? t : void 0;
		},
		getEnum(typeName) {
			const t = types.get(typeName);
			return (t === null || t === void 0 ? void 0 : t.kind) == "enum" ? t : void 0;
		},
		getExtension(typeName) {
			const t = types.get(typeName);
			return (t === null || t === void 0 ? void 0 : t.kind) == "extension" ? t : void 0;
		},
		getExtensionFor(extendee, no) {
			var _a;
			return (_a = extendees.get(extendee.typeName)) === null || _a === void 0 ? void 0 : _a.get(no);
		},
		getService(typeName) {
			const t = types.get(typeName);
			return (t === null || t === void 0 ? void 0 : t.kind) == "service" ? t : void 0;
		}
	};
}
const EDITION_PROTO2 = 998;
const EDITION_PROTO3 = 999;
const EDITION_UNSTABLE = 9999;
const TYPE_STRING = 9;
const TYPE_GROUP = 10;
const TYPE_MESSAGE = 11;
const TYPE_BYTES = 12;
const TYPE_ENUM = 14;
const LABEL_REPEATED = 3;
const LABEL_REQUIRED = 2;
const JS_STRING = 1;
const IDEMPOTENCY_UNKNOWN = 0;
const EXPLICIT = 1;
const IMPLICIT$3 = 2;
const LEGACY_REQUIRED$2 = 3;
const PACKED = 1;
const DELIMITED = 2;
const OPEN = 1;
const VERIFY = 2;
const maximumEdition = 1002;
const featureDefaults = {
	998: {
		fieldPresence: 1,
		enumType: 2,
		repeatedFieldEncoding: 2,
		utf8Validation: 3,
		messageEncoding: 1,
		jsonFormat: 2,
		enforceNamingStyle: 2,
		defaultSymbolVisibility: 1,
		enforceProtoLimits: 1
	},
	999: {
		fieldPresence: 2,
		enumType: 1,
		repeatedFieldEncoding: 1,
		utf8Validation: 2,
		messageEncoding: 1,
		jsonFormat: 1,
		enforceNamingStyle: 2,
		defaultSymbolVisibility: 1,
		enforceProtoLimits: 1
	},
	1e3: {
		fieldPresence: 1,
		enumType: 1,
		repeatedFieldEncoding: 1,
		utf8Validation: 2,
		messageEncoding: 1,
		jsonFormat: 1,
		enforceNamingStyle: 2,
		defaultSymbolVisibility: 1,
		enforceProtoLimits: 1
	},
	1001: {
		fieldPresence: 1,
		enumType: 1,
		repeatedFieldEncoding: 1,
		utf8Validation: 2,
		messageEncoding: 1,
		jsonFormat: 1,
		enforceNamingStyle: 1,
		defaultSymbolVisibility: 2,
		enforceProtoLimits: 1
	},
	1002: {
		fieldPresence: 1,
		enumType: 1,
		repeatedFieldEncoding: 1,
		utf8Validation: 2,
		messageEncoding: 1,
		jsonFormat: 1,
		enforceNamingStyle: 3,
		defaultSymbolVisibility: 4,
		enforceProtoLimits: 2
	}
};
/**
* Create a descriptor for a file, add it to the registry.
*/
function addFile(proto, reg) {
	var _a, _b;
	const file = {
		kind: "file",
		proto,
		deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
		edition: getFileEdition(proto),
		name: proto.name.replace(/\.proto$/, ""),
		dependencies: findFileDependencies(proto, reg),
		enums: [],
		messages: [],
		extensions: [],
		services: [],
		toString() {
			return `file ${proto.name}`;
		}
	};
	const mapEntriesStore = /* @__PURE__ */ new Map();
	const mapEntries = {
		get(typeName) {
			return mapEntriesStore.get(typeName);
		},
		add(desc) {
			var _a;
			assert(((_a = desc.proto.options) === null || _a === void 0 ? void 0 : _a.mapEntry) === true);
			mapEntriesStore.set(desc.typeName, desc);
		}
	};
	for (const enumProto of proto.enumType) addEnum(enumProto, file, void 0, reg);
	for (const messageProto of proto.messageType) addMessage(messageProto, file, void 0, reg, mapEntries);
	for (const serviceProto of proto.service) addService(serviceProto, file, reg);
	addExtensions(file, reg);
	for (const mapEntry of mapEntriesStore.values()) addFields(mapEntry, reg, mapEntries);
	for (const message of file.messages) {
		addFields(message, reg, mapEntries);
		addExtensions(message, reg);
	}
	reg.addFile(file, true);
}
/**
* Create descriptors for extensions, and add them to the message / file,
* and to our cart.
* Recurses into nested types.
*/
function addExtensions(desc, reg) {
	switch (desc.kind) {
		case "file":
			for (const proto of desc.proto.extension) {
				const ext = newField(proto, desc, reg);
				desc.extensions.push(ext);
				reg.add(ext);
			}
			break;
		case "message":
			for (const proto of desc.proto.extension) {
				const ext = newField(proto, desc, reg);
				desc.nestedExtensions.push(ext);
				reg.add(ext);
			}
			for (const message of desc.nestedMessages) addExtensions(message, reg);
	}
}
/**
* Create descriptors for fields and oneof groups, and add them to the message.
* Recurses into nested types.
*/
function addFields(message, reg, mapEntries) {
	const allOneofs = message.proto.oneofDecl.map((proto) => newOneof(proto, message));
	const oneofsSeen = /* @__PURE__ */ new Set();
	for (const proto of message.proto.field) {
		const oneof = findOneof(proto, allOneofs);
		const field = newField(proto, message, reg, oneof, mapEntries);
		message.fields.push(field);
		message.field[field.localName] = field;
		if (oneof === void 0) message.members.push(field);
		else {
			oneof.fields.push(field);
			if (!oneofsSeen.has(oneof)) {
				oneofsSeen.add(oneof);
				message.members.push(oneof);
			}
		}
	}
	for (const oneof of allOneofs.filter((o) => oneofsSeen.has(o))) message.oneofs.push(oneof);
	for (const child of message.nestedMessages) addFields(child, reg, mapEntries);
}
/**
* Create a descriptor for an enumeration, and add it our cart and to the
* parent type, if any.
*/
function addEnum(proto, file, parent, reg) {
	var _a, _b, _c, _d, _e, _f;
	const sharedPrefix = findEnumSharedPrefix(proto.name, proto.value);
	const desc = {
		kind: "enum",
		proto,
		deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
		file,
		parent,
		open: true,
		name: proto.name,
		typeName: makeTypeName(proto, parent, file),
		value: {},
		values: [],
		sharedPrefix,
		toString() {
			return `enum ${this.typeName}`;
		}
	};
	desc.open = isEnumOpen(desc);
	reg.add(desc);
	for (const p of proto.value) {
		const name = p.name;
		desc.values.push(desc.value[p.number] = {
			kind: "enum_value",
			proto: p,
			deprecated: (_d = (_c = p.options) === null || _c === void 0 ? void 0 : _c.deprecated) !== null && _d !== void 0 ? _d : false,
			parent: desc,
			name,
			localName: safeObjectProperty(sharedPrefix == void 0 ? name : name.substring(sharedPrefix.length)),
			number: p.number,
			jsonName: (_e = findEnumValueJsonName(p)) !== null && _e !== void 0 ? _e : name,
			toString() {
				return `enum value ${desc.typeName}.${name}`;
			}
		});
	}
	((_f = parent === null || parent === void 0 ? void 0 : parent.nestedEnums) !== null && _f !== void 0 ? _f : file.enums).push(desc);
}
/**
* Create a descriptor for a message, including nested types, and add it to our
* cart. Note that this does not create descriptors fields.
*/
function addMessage(proto, file, parent, reg, mapEntries) {
	var _a, _b, _c, _d;
	const desc = {
		kind: "message",
		proto,
		deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
		file,
		parent,
		name: proto.name,
		typeName: makeTypeName(proto, parent, file),
		fields: [],
		field: {},
		oneofs: [],
		members: [],
		nestedEnums: [],
		nestedMessages: [],
		nestedExtensions: [],
		toString() {
			return `message ${this.typeName}`;
		}
	};
	if (((_c = proto.options) === null || _c === void 0 ? void 0 : _c.mapEntry) === true) mapEntries.add(desc);
	else {
		((_d = parent === null || parent === void 0 ? void 0 : parent.nestedMessages) !== null && _d !== void 0 ? _d : file.messages).push(desc);
		reg.add(desc);
	}
	for (const enumProto of proto.enumType) addEnum(enumProto, file, desc, reg);
	for (const messageProto of proto.nestedType) addMessage(messageProto, file, desc, reg, mapEntries);
}
/**
* Create a descriptor for a service, including methods, and add it to our
* cart.
*/
function addService(proto, file, reg) {
	var _a, _b;
	const desc = {
		kind: "service",
		proto,
		deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
		file,
		name: proto.name,
		typeName: makeTypeName(proto, void 0, file),
		methods: [],
		method: {},
		toString() {
			return `service ${this.typeName}`;
		}
	};
	file.services.push(desc);
	reg.add(desc);
	for (const methodProto of proto.method) {
		const method = newMethod(methodProto, desc, reg);
		desc.methods.push(method);
		desc.method[method.localName] = method;
	}
}
/**
* Create a descriptor for a method.
*/
function newMethod(proto, parent, reg) {
	var _a, _b, _c, _d;
	let methodKind;
	if (proto.clientStreaming && proto.serverStreaming) methodKind = "bidi_streaming";
	else if (proto.clientStreaming) methodKind = "client_streaming";
	else if (proto.serverStreaming) methodKind = "server_streaming";
	else methodKind = "unary";
	const input = reg.getMessage(trimLeadingDot(proto.inputType));
	const output = reg.getMessage(trimLeadingDot(proto.outputType));
	assert(input, `invalid MethodDescriptorProto: input_type ${proto.inputType} not found`);
	assert(output, `invalid MethodDescriptorProto: output_type ${proto.inputType} not found`);
	const name = proto.name;
	return {
		kind: "rpc",
		proto,
		deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
		parent,
		name,
		localName: safeObjectProperty(name.length ? safeObjectProperty(name[0].toLowerCase() + name.substring(1)) : name),
		methodKind,
		input,
		output,
		idempotency: (_d = (_c = proto.options) === null || _c === void 0 ? void 0 : _c.idempotencyLevel) !== null && _d !== void 0 ? _d : IDEMPOTENCY_UNKNOWN,
		toString() {
			return `rpc ${parent.typeName}.${name}`;
		}
	};
}
/**
* Create a descriptor for a oneof group.
*/
function newOneof(proto, parent) {
	return {
		kind: "oneof",
		proto,
		deprecated: false,
		parent,
		fields: [],
		name: proto.name,
		localName: safeObjectProperty(protoCamelCase(proto.name)),
		toString() {
			return `oneof ${parent.typeName}.${this.name}`;
		}
	};
}
function newField(proto, parentOrFile, reg, oneof, mapEntries) {
	var _a, _b, _c;
	const isExtension = mapEntries === void 0;
	const field = {
		kind: "field",
		proto,
		deprecated: (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.deprecated) !== null && _b !== void 0 ? _b : false,
		name: proto.name,
		number: proto.number,
		scalar: void 0,
		message: void 0,
		enum: void 0,
		presence: getFieldPresence(proto, oneof, isExtension, parentOrFile),
		utf8Validation: isUtf8Validated(proto, parentOrFile),
		listKind: void 0,
		mapKind: void 0,
		mapKey: void 0,
		delimitedEncoding: void 0,
		packed: void 0,
		longAsString: false,
		getDefaultValue: void 0
	};
	let toStr;
	if (isExtension) {
		const file = parentOrFile.kind == "file" ? parentOrFile : parentOrFile.file;
		const parent = parentOrFile.kind == "file" ? void 0 : parentOrFile;
		const typeName = makeTypeName(proto, parent, file);
		field.kind = "extension";
		field.file = file;
		field.parent = parent;
		field.oneof = void 0;
		field.typeName = typeName;
		field.jsonName = `[${typeName}]`;
		toStr = () => `extension ${typeName}`;
		const extendee = reg.getMessage(trimLeadingDot(proto.extendee));
		assert(extendee, `invalid FieldDescriptorProto: extendee ${proto.extendee} not found`);
		field.extendee = extendee;
	} else {
		const parent = parentOrFile;
		assert(parent.kind == "message");
		field.parent = parent;
		field.oneof = oneof;
		field.localName = oneof ? protoCamelCase(proto.name) : safeObjectProperty(protoCamelCase(proto.name));
		field.jsonName = proto.jsonName;
		toStr = () => `field ${parent.typeName}.${proto.name}`;
	}
	Object.defineProperty(field, "toString", {
		value: toStr,
		writable: true,
		enumerable: true,
		configurable: true
	});
	const label = proto.label;
	const type = proto.type;
	const jstype = (_c = proto.options) === null || _c === void 0 ? void 0 : _c.jstype;
	if (label === LABEL_REPEATED) {
		const mapEntry = type == TYPE_MESSAGE ? mapEntries === null || mapEntries === void 0 ? void 0 : mapEntries.get(trimLeadingDot(proto.typeName)) : void 0;
		if (mapEntry) {
			field.fieldKind = "map";
			const { key, value } = findMapEntryFields(mapEntry);
			field.mapKey = key.scalar;
			field.mapKind = value.fieldKind;
			field.message = value.message;
			field.delimitedEncoding = false;
			field.enum = value.enum;
			field.scalar = value.scalar;
			return field;
		}
		field.fieldKind = "list";
		switch (type) {
			case TYPE_MESSAGE:
			case TYPE_GROUP:
				field.listKind = "message";
				field.message = reg.getMessage(trimLeadingDot(proto.typeName));
				assert(field.message);
				field.delimitedEncoding = isDelimitedEncoding(proto, parentOrFile);
				break;
			case TYPE_ENUM:
				field.listKind = "enum";
				field.enum = reg.getEnum(trimLeadingDot(proto.typeName));
				assert(field.enum);
				break;
			default:
				field.listKind = "scalar";
				field.scalar = type;
				field.longAsString = jstype == JS_STRING;
		}
		field.packed = isPackedField(proto, parentOrFile);
		return field;
	}
	switch (type) {
		case TYPE_MESSAGE:
		case TYPE_GROUP:
			field.fieldKind = "message";
			field.message = reg.getMessage(trimLeadingDot(proto.typeName));
			assert(field.message, `invalid FieldDescriptorProto: type_name ${proto.typeName} not found`);
			field.delimitedEncoding = isDelimitedEncoding(proto, parentOrFile);
			field.getDefaultValue = () => void 0;
			break;
		case TYPE_ENUM: {
			const enumeration = reg.getEnum(trimLeadingDot(proto.typeName));
			assert(enumeration !== void 0, `invalid FieldDescriptorProto: type_name ${proto.typeName} not found`);
			field.fieldKind = "enum";
			field.enum = reg.getEnum(trimLeadingDot(proto.typeName));
			field.getDefaultValue = () => {
				return unsafeIsSetExplicit(proto, "defaultValue") ? parseTextFormatEnumValue(enumeration, proto.defaultValue) : void 0;
			};
			break;
		}
		default:
			field.fieldKind = "scalar";
			field.scalar = type;
			field.longAsString = jstype == JS_STRING;
			field.getDefaultValue = () => {
				return unsafeIsSetExplicit(proto, "defaultValue") ? parseTextFormatScalarValue(type, proto.defaultValue) : void 0;
			};
	}
	return field;
}
/**
* Parse the "syntax" and "edition" fields, returning one of the supported
* editions.
*/
function getFileEdition(proto) {
	switch (proto.syntax) {
		case "":
		case "proto2": return EDITION_PROTO2;
		case "proto3": return EDITION_PROTO3;
		case "editions":
			if (proto.edition === EDITION_UNSTABLE) return maximumEdition;
			if (proto.edition in featureDefaults) return proto.edition;
			throw new Error(`${proto.name}: unsupported edition`);
		default: throw new Error(`${proto.name}: unsupported syntax "${proto.syntax}"`);
	}
}
/**
* Resolve dependencies of FileDescriptorProto to DescFile.
*/
function findFileDependencies(proto, reg) {
	return proto.dependency.map((wantName) => {
		const dep = reg.getFile(wantName);
		if (!dep) throw new Error(`Cannot find ${wantName}, imported by ${proto.name}`);
		return dep;
	});
}
/**
* Finds a prefix shared by enum values, for example `my_enum_` for
* `enum MyEnum {MY_ENUM_A=0; MY_ENUM_B=1;}`.
*/
function findEnumSharedPrefix(enumName, values) {
	const prefix = camelToSnakeCase(enumName) + "_";
	for (const value of values) {
		if (!value.name.toLowerCase().startsWith(prefix)) return;
		const shortName = value.name.substring(prefix.length);
		if (shortName.length == 0) return;
		if (/^\d/.test(shortName)) return;
	}
	return prefix;
}
/**
* Finds the custom JSON name of an enum value, set with the option
* `(pb.enumvalue.json).string` from google/protobuf/json_enumvalue_options.proto.
*
* Extensions are stored as unknown fields, so we read the option from the
* wire format instead of importing the extension, which depends on this file.
*/
function findEnumValueJsonName(proto) {
	var _a, _b;
	const extensionNumber = 998;
	const fieldNumber = 1;
	let jsonName;
	for (const uf of (_b = (_a = proto.options) === null || _a === void 0 ? void 0 : _a.$unknown) !== null && _b !== void 0 ? _b : []) {
		if (uf.no !== extensionNumber || uf.wireType !== WireType.LengthDelimited) continue;
		const reader = new BinaryReader(new BinaryReader(uf.data).bytes());
		while (reader.pos < reader.len) {
			const [no, wireType] = reader.tag();
			if (no === fieldNumber && wireType === WireType.LengthDelimited) jsonName = reader.string();
			else reader.skip(wireType, no);
		}
	}
	return jsonName;
}
/**
* Converts lowerCamelCase or UpperCamelCase into lower_snake_case.
* This is used to find shared prefixes in an enum.
*/
function camelToSnakeCase(camel) {
	return (camel.substring(0, 1) + camel.substring(1).replace(/[A-Z]/g, (c) => "_" + c)).toLowerCase();
}
/**
* Create a fully qualified name for a protobuf type or extension field.
*
* The fully qualified name for messages, enumerations, and services is
* constructed by concatenating the package name (if present), parent
* message names (for nested types), and the type name. We omit the leading
* dot added by protobuf compilers. Examples:
* - mypackage.MyMessage
* - mypackage.MyMessage.NestedMessage
*
* The fully qualified name for extension fields is constructed by
* concatenating the package name (if present), parent message names (for
* extensions declared within a message), and the field name. Examples:
* - mypackage.extfield
* - mypackage.MyMessage.extfield
*/
function makeTypeName(proto, parent, file) {
	let typeName;
	if (parent) typeName = `${parent.typeName}.${proto.name}`;
	else if (file.proto.package.length > 0) typeName = `${file.proto.package}.${proto.name}`;
	else typeName = `${proto.name}`;
	return typeName;
}
/**
* Remove the leading dot from a fully qualified type name.
*/
function trimLeadingDot(typeName) {
	return typeName.startsWith(".") ? typeName.substring(1) : typeName;
}
/**
* Did the user put the field in a oneof group?
* Synthetic oneofs for proto3 optionals are ignored.
*/
function findOneof(proto, allOneofs) {
	if (!unsafeIsSetExplicit(proto, "oneofIndex")) return;
	if (proto.proto3Optional) return;
	const oneof = allOneofs[proto.oneofIndex];
	assert(oneof, `invalid FieldDescriptorProto: oneof #${proto.oneofIndex} for field #${proto.number} not found`);
	return oneof;
}
/**
* Presence of the field.
* See https://protobuf.dev/programming-guides/field_presence/
*/
function getFieldPresence(proto, oneof, isExtension, parent) {
	if (proto.label == LABEL_REQUIRED) return LEGACY_REQUIRED$2;
	if (proto.label == LABEL_REPEATED) return IMPLICIT$3;
	if (!!oneof || proto.proto3Optional) return EXPLICIT;
	if (isExtension) return EXPLICIT;
	const resolved = resolveFeature("fieldPresence", {
		proto,
		parent
	});
	if (resolved == IMPLICIT$3 && (proto.type == TYPE_MESSAGE || proto.type == TYPE_GROUP)) return EXPLICIT;
	return resolved;
}
/**
* Pack this repeated field?
*/
function isPackedField(proto, parent) {
	if (proto.label != LABEL_REPEATED) return false;
	switch (proto.type) {
		case TYPE_STRING:
		case TYPE_BYTES:
		case TYPE_GROUP:
		case TYPE_MESSAGE: return false;
	}
	const o = proto.options;
	if (o && unsafeIsSetExplicit(o, "packed")) return o.packed;
	return PACKED == resolveFeature("repeatedFieldEncoding", {
		proto,
		parent
	});
}
/**
* Find the key and value fields of a synthetic map entry message.
*/
function findMapEntryFields(mapEntry) {
	const key = mapEntry.fields.find((f) => f.number === 1);
	const value = mapEntry.fields.find((f) => f.number === 2);
	assert(key && key.fieldKind == "scalar" && key.scalar != ScalarType.BYTES && key.scalar != ScalarType.FLOAT && key.scalar != ScalarType.DOUBLE && value && value.fieldKind != "list" && value.fieldKind != "map");
	return {
		key,
		value
	};
}
/**
* Enumerations can be open or closed.
* See https://protobuf.dev/programming-guides/enum/
*/
function isEnumOpen(desc) {
	var _a;
	return OPEN == resolveFeature("enumType", {
		proto: desc.proto,
		parent: (_a = desc.parent) !== null && _a !== void 0 ? _a : desc.file
	});
}
/**
* Encode the message delimited (a.k.a. proto2 group encoding), or
* length-prefixed?
*/
function isDelimitedEncoding(proto, parent) {
	if (proto.type == TYPE_GROUP) return true;
	return DELIMITED == resolveFeature("messageEncoding", {
		proto,
		parent
	});
}
/**
* Reject invalid UTF-8 when reading string fields from the binary wire format?
* Driven by the resolved `utf8_validation` feature: VERIFY (proto3 / editions
* 2023+ default) enforces; NONE (proto2 default) does not.
*/
function isUtf8Validated(proto, parent) {
	return VERIFY == resolveFeature("utf8Validation", {
		proto,
		parent
	});
}
function resolveFeature(name, ref) {
	var _a, _b;
	const featureSet = (_a = ref.proto.options) === null || _a === void 0 ? void 0 : _a.features;
	if (featureSet) {
		const val = featureSet[name];
		if (val != 0) return val;
	}
	if ("kind" in ref) {
		if (ref.kind == "message") return resolveFeature(name, (_b = ref.parent) !== null && _b !== void 0 ? _b : ref.file);
		const editionDefaults = featureDefaults[ref.edition];
		if (!editionDefaults) throw new Error(`feature default for edition ${ref.edition} not found`);
		return editionDefaults[name];
	}
	return resolveFeature(name, ref.parent);
}
/**
* Assert that condition is truthy or throw error (with message)
*/
function assert(condition, msg) {
	if (!condition) throw new Error(msg);
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/boot.js
/**
* Hydrate a file descriptor for google/protobuf/descriptor.proto from a plain
* object.
*
* See createFileDescriptorProtoBoot() for details.
*
* @private
*/
function boot(boot) {
	const root = bootFileDescriptorProto(boot);
	root.messageType.forEach(restoreJsonNames);
	return createFileRegistry(root, () => void 0).getFile(root.name);
}
/**
* Creates the message google.protobuf.FileDescriptorProto from an object literal.
*
* See createFileDescriptorProtoBoot() for details.
*
* @private
*/
function bootFileDescriptorProto(init) {
	return Object.assign(Object.create({
		syntax: "",
		edition: 0
	}), Object.assign(Object.assign({
		$typeName: "google.protobuf.FileDescriptorProto",
		dependency: [],
		publicDependency: [],
		weakDependency: [],
		optionDependency: [],
		service: [],
		extension: []
	}, init), {
		messageType: init.messageType.map(bootDescriptorProto),
		enumType: init.enumType.map(bootEnumDescriptorProto)
	}));
}
function bootDescriptorProto(init) {
	var _a, _b, _c, _d, _e, _f, _g, _h;
	return Object.assign(Object.create({ visibility: 0 }), {
		$typeName: "google.protobuf.DescriptorProto",
		name: init.name,
		field: (_b = (_a = init.field) === null || _a === void 0 ? void 0 : _a.map(bootFieldDescriptorProto)) !== null && _b !== void 0 ? _b : [],
		extension: [],
		nestedType: (_d = (_c = init.nestedType) === null || _c === void 0 ? void 0 : _c.map(bootDescriptorProto)) !== null && _d !== void 0 ? _d : [],
		enumType: (_f = (_e = init.enumType) === null || _e === void 0 ? void 0 : _e.map(bootEnumDescriptorProto)) !== null && _f !== void 0 ? _f : [],
		extensionRange: (_h = (_g = init.extensionRange) === null || _g === void 0 ? void 0 : _g.map((e) => Object.assign({ $typeName: "google.protobuf.DescriptorProto.ExtensionRange" }, e))) !== null && _h !== void 0 ? _h : [],
		oneofDecl: [],
		reservedRange: [],
		reservedName: []
	});
}
function bootFieldDescriptorProto(init) {
	return Object.assign(Object.create({
		label: 1,
		typeName: "",
		extendee: "",
		defaultValue: "",
		oneofIndex: 0,
		jsonName: "",
		proto3Optional: false
	}), Object.assign(Object.assign({ $typeName: "google.protobuf.FieldDescriptorProto" }, init), { options: init.options ? bootFieldOptions(init.options) : void 0 }));
}
function bootFieldOptions(init) {
	var _a, _b, _c;
	return Object.assign(Object.create({
		ctype: 0,
		packed: false,
		jstype: 0,
		lazy: false,
		unverifiedLazy: false,
		deprecated: false,
		weak: false,
		debugRedact: false,
		retention: 0
	}), Object.assign(Object.assign({ $typeName: "google.protobuf.FieldOptions" }, init), {
		targets: (_a = init.targets) !== null && _a !== void 0 ? _a : [],
		editionDefaults: (_c = (_b = init.editionDefaults) === null || _b === void 0 ? void 0 : _b.map((e) => Object.assign({ $typeName: "google.protobuf.FieldOptions.EditionDefault" }, e))) !== null && _c !== void 0 ? _c : [],
		uninterpretedOption: []
	}));
}
function bootEnumDescriptorProto(init) {
	return Object.assign(Object.create({ visibility: 0 }), {
		$typeName: "google.protobuf.EnumDescriptorProto",
		name: init.name,
		reservedName: [],
		reservedRange: [],
		value: init.value.map((e) => Object.assign({ $typeName: "google.protobuf.EnumValueDescriptorProto" }, e))
	});
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/message.js
/**
* Hydrate a message descriptor.
*
* @private
*/
function messageDesc(file, path, ...paths) {
	return paths.reduce((acc, cur) => acc.nestedMessages[cur], file.messages[path]);
}
/**
* Describes the message google.protobuf.FileDescriptorProto.
* Use `create(FileDescriptorProtoSchema)` to create a new message.
*/
const FileDescriptorProtoSchema = /*@__PURE__*/ messageDesc(/* @__PURE__ */ boot({
	"name": "google/protobuf/descriptor.proto",
	"package": "google.protobuf",
	"messageType": [
		{
			"name": "FileDescriptorSet",
			"field": [{
				"name": "file",
				"number": 1,
				"type": 11,
				"label": 3,
				"typeName": ".google.protobuf.FileDescriptorProto"
			}],
			"extensionRange": [{
				"start": 536e6,
				"end": 536000001
			}]
		},
		{
			"name": "FileDescriptorProto",
			"field": [
				{
					"name": "name",
					"number": 1,
					"type": 9,
					"label": 1
				},
				{
					"name": "package",
					"number": 2,
					"type": 9,
					"label": 1
				},
				{
					"name": "dependency",
					"number": 3,
					"type": 9,
					"label": 3
				},
				{
					"name": "public_dependency",
					"number": 10,
					"type": 5,
					"label": 3
				},
				{
					"name": "weak_dependency",
					"number": 11,
					"type": 5,
					"label": 3
				},
				{
					"name": "option_dependency",
					"number": 15,
					"type": 9,
					"label": 3
				},
				{
					"name": "message_type",
					"number": 4,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.DescriptorProto"
				},
				{
					"name": "enum_type",
					"number": 5,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.EnumDescriptorProto"
				},
				{
					"name": "service",
					"number": 6,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.ServiceDescriptorProto"
				},
				{
					"name": "extension",
					"number": 7,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.FieldDescriptorProto"
				},
				{
					"name": "options",
					"number": 8,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.FileOptions"
				},
				{
					"name": "source_code_info",
					"number": 9,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.SourceCodeInfo"
				},
				{
					"name": "syntax",
					"number": 12,
					"type": 9,
					"label": 1
				},
				{
					"name": "edition",
					"number": 14,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.Edition"
				}
			]
		},
		{
			"name": "DescriptorProto",
			"field": [
				{
					"name": "name",
					"number": 1,
					"type": 9,
					"label": 1
				},
				{
					"name": "field",
					"number": 2,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.FieldDescriptorProto"
				},
				{
					"name": "extension",
					"number": 6,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.FieldDescriptorProto"
				},
				{
					"name": "nested_type",
					"number": 3,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.DescriptorProto"
				},
				{
					"name": "enum_type",
					"number": 4,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.EnumDescriptorProto"
				},
				{
					"name": "extension_range",
					"number": 5,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.DescriptorProto.ExtensionRange"
				},
				{
					"name": "oneof_decl",
					"number": 8,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.OneofDescriptorProto"
				},
				{
					"name": "options",
					"number": 7,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.MessageOptions"
				},
				{
					"name": "reserved_range",
					"number": 9,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.DescriptorProto.ReservedRange"
				},
				{
					"name": "reserved_name",
					"number": 10,
					"type": 9,
					"label": 3
				},
				{
					"name": "visibility",
					"number": 11,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.SymbolVisibility"
				}
			],
			"nestedType": [{
				"name": "ExtensionRange",
				"field": [
					{
						"name": "start",
						"number": 1,
						"type": 5,
						"label": 1
					},
					{
						"name": "end",
						"number": 2,
						"type": 5,
						"label": 1
					},
					{
						"name": "options",
						"number": 3,
						"type": 11,
						"label": 1,
						"typeName": ".google.protobuf.ExtensionRangeOptions"
					}
				]
			}, {
				"name": "ReservedRange",
				"field": [{
					"name": "start",
					"number": 1,
					"type": 5,
					"label": 1
				}, {
					"name": "end",
					"number": 2,
					"type": 5,
					"label": 1
				}]
			}]
		},
		{
			"name": "ExtensionRangeOptions",
			"field": [
				{
					"name": "uninterpreted_option",
					"number": 999,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.UninterpretedOption"
				},
				{
					"name": "declaration",
					"number": 2,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.ExtensionRangeOptions.Declaration",
					"options": { "retention": 2 }
				},
				{
					"name": "features",
					"number": 50,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet"
				},
				{
					"name": "verification",
					"number": 3,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.ExtensionRangeOptions.VerificationState",
					"defaultValue": "UNVERIFIED",
					"options": { "retention": 2 }
				}
			],
			"nestedType": [{
				"name": "Declaration",
				"field": [
					{
						"name": "number",
						"number": 1,
						"type": 5,
						"label": 1
					},
					{
						"name": "full_name",
						"number": 2,
						"type": 9,
						"label": 1
					},
					{
						"name": "type",
						"number": 3,
						"type": 9,
						"label": 1
					},
					{
						"name": "reserved",
						"number": 5,
						"type": 8,
						"label": 1
					},
					{
						"name": "repeated",
						"number": 6,
						"type": 8,
						"label": 1
					}
				]
			}],
			"enumType": [{
				"name": "VerificationState",
				"value": [{
					"name": "DECLARATION",
					"number": 0
				}, {
					"name": "UNVERIFIED",
					"number": 1
				}]
			}],
			"extensionRange": [{
				"start": 990,
				"end": 999
			}, {
				"start": 1e3,
				"end": 536870912
			}]
		},
		{
			"name": "FieldDescriptorProto",
			"field": [
				{
					"name": "name",
					"number": 1,
					"type": 9,
					"label": 1
				},
				{
					"name": "number",
					"number": 3,
					"type": 5,
					"label": 1
				},
				{
					"name": "label",
					"number": 4,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.FieldDescriptorProto.Label"
				},
				{
					"name": "type",
					"number": 5,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.FieldDescriptorProto.Type"
				},
				{
					"name": "type_name",
					"number": 6,
					"type": 9,
					"label": 1
				},
				{
					"name": "extendee",
					"number": 2,
					"type": 9,
					"label": 1
				},
				{
					"name": "default_value",
					"number": 7,
					"type": 9,
					"label": 1
				},
				{
					"name": "oneof_index",
					"number": 9,
					"type": 5,
					"label": 1
				},
				{
					"name": "json_name",
					"number": 10,
					"type": 9,
					"label": 1
				},
				{
					"name": "options",
					"number": 8,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.FieldOptions"
				},
				{
					"name": "proto3_optional",
					"number": 17,
					"type": 8,
					"label": 1
				}
			],
			"enumType": [{
				"name": "Type",
				"value": [
					{
						"name": "TYPE_DOUBLE",
						"number": 1
					},
					{
						"name": "TYPE_FLOAT",
						"number": 2
					},
					{
						"name": "TYPE_INT64",
						"number": 3
					},
					{
						"name": "TYPE_UINT64",
						"number": 4
					},
					{
						"name": "TYPE_INT32",
						"number": 5
					},
					{
						"name": "TYPE_FIXED64",
						"number": 6
					},
					{
						"name": "TYPE_FIXED32",
						"number": 7
					},
					{
						"name": "TYPE_BOOL",
						"number": 8
					},
					{
						"name": "TYPE_STRING",
						"number": 9
					},
					{
						"name": "TYPE_GROUP",
						"number": 10
					},
					{
						"name": "TYPE_MESSAGE",
						"number": 11
					},
					{
						"name": "TYPE_BYTES",
						"number": 12
					},
					{
						"name": "TYPE_UINT32",
						"number": 13
					},
					{
						"name": "TYPE_ENUM",
						"number": 14
					},
					{
						"name": "TYPE_SFIXED32",
						"number": 15
					},
					{
						"name": "TYPE_SFIXED64",
						"number": 16
					},
					{
						"name": "TYPE_SINT32",
						"number": 17
					},
					{
						"name": "TYPE_SINT64",
						"number": 18
					}
				]
			}, {
				"name": "Label",
				"value": [
					{
						"name": "LABEL_OPTIONAL",
						"number": 1
					},
					{
						"name": "LABEL_REPEATED",
						"number": 3
					},
					{
						"name": "LABEL_REQUIRED",
						"number": 2
					}
				]
			}]
		},
		{
			"name": "OneofDescriptorProto",
			"field": [{
				"name": "name",
				"number": 1,
				"type": 9,
				"label": 1
			}, {
				"name": "options",
				"number": 2,
				"type": 11,
				"label": 1,
				"typeName": ".google.protobuf.OneofOptions"
			}]
		},
		{
			"name": "EnumDescriptorProto",
			"field": [
				{
					"name": "name",
					"number": 1,
					"type": 9,
					"label": 1
				},
				{
					"name": "value",
					"number": 2,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.EnumValueDescriptorProto"
				},
				{
					"name": "options",
					"number": 3,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.EnumOptions"
				},
				{
					"name": "reserved_range",
					"number": 4,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.EnumDescriptorProto.EnumReservedRange"
				},
				{
					"name": "reserved_name",
					"number": 5,
					"type": 9,
					"label": 3
				},
				{
					"name": "visibility",
					"number": 6,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.SymbolVisibility"
				}
			],
			"nestedType": [{
				"name": "EnumReservedRange",
				"field": [{
					"name": "start",
					"number": 1,
					"type": 5,
					"label": 1
				}, {
					"name": "end",
					"number": 2,
					"type": 5,
					"label": 1
				}]
			}]
		},
		{
			"name": "EnumValueDescriptorProto",
			"field": [
				{
					"name": "name",
					"number": 1,
					"type": 9,
					"label": 1
				},
				{
					"name": "number",
					"number": 2,
					"type": 5,
					"label": 1
				},
				{
					"name": "options",
					"number": 3,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.EnumValueOptions"
				}
			]
		},
		{
			"name": "ServiceDescriptorProto",
			"field": [
				{
					"name": "name",
					"number": 1,
					"type": 9,
					"label": 1
				},
				{
					"name": "method",
					"number": 2,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.MethodDescriptorProto"
				},
				{
					"name": "options",
					"number": 3,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.ServiceOptions"
				}
			]
		},
		{
			"name": "MethodDescriptorProto",
			"field": [
				{
					"name": "name",
					"number": 1,
					"type": 9,
					"label": 1
				},
				{
					"name": "input_type",
					"number": 2,
					"type": 9,
					"label": 1
				},
				{
					"name": "output_type",
					"number": 3,
					"type": 9,
					"label": 1
				},
				{
					"name": "options",
					"number": 4,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.MethodOptions"
				},
				{
					"name": "client_streaming",
					"number": 5,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "server_streaming",
					"number": 6,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				}
			]
		},
		{
			"name": "FileOptions",
			"field": [
				{
					"name": "java_package",
					"number": 1,
					"type": 9,
					"label": 1
				},
				{
					"name": "java_outer_classname",
					"number": 8,
					"type": 9,
					"label": 1
				},
				{
					"name": "java_multiple_files",
					"number": 10,
					"type": 8,
					"label": 1,
					"defaultValue": "false",
					"options": {}
				},
				{
					"name": "java_generate_equals_and_hash",
					"number": 20,
					"type": 8,
					"label": 1,
					"options": { "deprecated": true }
				},
				{
					"name": "java_string_check_utf8",
					"number": 27,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "optimize_for",
					"number": 9,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.FileOptions.OptimizeMode",
					"defaultValue": "SPEED"
				},
				{
					"name": "go_package",
					"number": 11,
					"type": 9,
					"label": 1
				},
				{
					"name": "cc_generic_services",
					"number": 16,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "java_generic_services",
					"number": 17,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "py_generic_services",
					"number": 18,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "deprecated",
					"number": 23,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "cc_enable_arenas",
					"number": 31,
					"type": 8,
					"label": 1,
					"defaultValue": "true",
					"options": {}
				},
				{
					"name": "objc_class_prefix",
					"number": 36,
					"type": 9,
					"label": 1
				},
				{
					"name": "csharp_namespace",
					"number": 37,
					"type": 9,
					"label": 1
				},
				{
					"name": "swift_prefix",
					"number": 39,
					"type": 9,
					"label": 1
				},
				{
					"name": "php_class_prefix",
					"number": 40,
					"type": 9,
					"label": 1
				},
				{
					"name": "php_namespace",
					"number": 41,
					"type": 9,
					"label": 1
				},
				{
					"name": "php_metadata_namespace",
					"number": 44,
					"type": 9,
					"label": 1
				},
				{
					"name": "ruby_package",
					"number": 45,
					"type": 9,
					"label": 1
				},
				{
					"name": "features",
					"number": 50,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet"
				},
				{
					"name": "uninterpreted_option",
					"number": 999,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.UninterpretedOption"
				}
			],
			"enumType": [{
				"name": "OptimizeMode",
				"value": [
					{
						"name": "SPEED",
						"number": 1
					},
					{
						"name": "CODE_SIZE",
						"number": 2
					},
					{
						"name": "LITE_RUNTIME",
						"number": 3
					}
				]
			}],
			"extensionRange": [{
				"start": 990,
				"end": 999
			}, {
				"start": 1e3,
				"end": 536870912
			}]
		},
		{
			"name": "MessageOptions",
			"field": [
				{
					"name": "message_set_wire_format",
					"number": 1,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "no_standard_descriptor_accessor",
					"number": 2,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "deprecated",
					"number": 3,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "map_entry",
					"number": 7,
					"type": 8,
					"label": 1
				},
				{
					"name": "deprecated_legacy_json_field_conflicts",
					"number": 11,
					"type": 8,
					"label": 1,
					"options": { "deprecated": true }
				},
				{
					"name": "features",
					"number": 12,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet"
				},
				{
					"name": "uninterpreted_option",
					"number": 999,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.UninterpretedOption"
				}
			],
			"extensionRange": [{
				"start": 990,
				"end": 999
			}, {
				"start": 1e3,
				"end": 536870912
			}]
		},
		{
			"name": "FieldOptions",
			"field": [
				{
					"name": "ctype",
					"number": 1,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.FieldOptions.CType",
					"defaultValue": "STRING"
				},
				{
					"name": "packed",
					"number": 2,
					"type": 8,
					"label": 1
				},
				{
					"name": "jstype",
					"number": 6,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.FieldOptions.JSType",
					"defaultValue": "JS_NORMAL"
				},
				{
					"name": "lazy",
					"number": 5,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "unverified_lazy",
					"number": 15,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "deprecated",
					"number": 3,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "weak",
					"number": 10,
					"type": 8,
					"label": 1,
					"defaultValue": "false",
					"options": { "deprecated": true }
				},
				{
					"name": "debug_redact",
					"number": 16,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "retention",
					"number": 17,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.FieldOptions.OptionRetention"
				},
				{
					"name": "targets",
					"number": 19,
					"type": 14,
					"label": 3,
					"typeName": ".google.protobuf.FieldOptions.OptionTargetType"
				},
				{
					"name": "edition_defaults",
					"number": 20,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.FieldOptions.EditionDefault"
				},
				{
					"name": "features",
					"number": 21,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet"
				},
				{
					"name": "feature_support",
					"number": 22,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.FieldOptions.FeatureSupport"
				},
				{
					"name": "uninterpreted_option",
					"number": 999,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.UninterpretedOption"
				}
			],
			"nestedType": [{
				"name": "EditionDefault",
				"field": [{
					"name": "edition",
					"number": 3,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.Edition"
				}, {
					"name": "value",
					"number": 2,
					"type": 9,
					"label": 1
				}]
			}, {
				"name": "FeatureSupport",
				"field": [
					{
						"name": "edition_introduced",
						"number": 1,
						"type": 14,
						"label": 1,
						"typeName": ".google.protobuf.Edition"
					},
					{
						"name": "edition_deprecated",
						"number": 2,
						"type": 14,
						"label": 1,
						"typeName": ".google.protobuf.Edition"
					},
					{
						"name": "deprecation_warning",
						"number": 3,
						"type": 9,
						"label": 1
					},
					{
						"name": "edition_removed",
						"number": 4,
						"type": 14,
						"label": 1,
						"typeName": ".google.protobuf.Edition"
					},
					{
						"name": "removal_error",
						"number": 5,
						"type": 9,
						"label": 1
					}
				]
			}],
			"enumType": [
				{
					"name": "CType",
					"value": [
						{
							"name": "STRING",
							"number": 0
						},
						{
							"name": "CORD",
							"number": 1
						},
						{
							"name": "STRING_PIECE",
							"number": 2
						}
					]
				},
				{
					"name": "JSType",
					"value": [
						{
							"name": "JS_NORMAL",
							"number": 0
						},
						{
							"name": "JS_STRING",
							"number": 1
						},
						{
							"name": "JS_NUMBER",
							"number": 2
						}
					]
				},
				{
					"name": "OptionRetention",
					"value": [
						{
							"name": "RETENTION_UNKNOWN",
							"number": 0
						},
						{
							"name": "RETENTION_RUNTIME",
							"number": 1
						},
						{
							"name": "RETENTION_SOURCE",
							"number": 2
						}
					]
				},
				{
					"name": "OptionTargetType",
					"value": [
						{
							"name": "TARGET_TYPE_UNKNOWN",
							"number": 0
						},
						{
							"name": "TARGET_TYPE_FILE",
							"number": 1
						},
						{
							"name": "TARGET_TYPE_EXTENSION_RANGE",
							"number": 2
						},
						{
							"name": "TARGET_TYPE_MESSAGE",
							"number": 3
						},
						{
							"name": "TARGET_TYPE_FIELD",
							"number": 4
						},
						{
							"name": "TARGET_TYPE_ONEOF",
							"number": 5
						},
						{
							"name": "TARGET_TYPE_ENUM",
							"number": 6
						},
						{
							"name": "TARGET_TYPE_ENUM_ENTRY",
							"number": 7
						},
						{
							"name": "TARGET_TYPE_SERVICE",
							"number": 8
						},
						{
							"name": "TARGET_TYPE_METHOD",
							"number": 9
						}
					]
				}
			],
			"extensionRange": [{
				"start": 990,
				"end": 999
			}, {
				"start": 1e3,
				"end": 536870912
			}]
		},
		{
			"name": "OneofOptions",
			"field": [{
				"name": "features",
				"number": 1,
				"type": 11,
				"label": 1,
				"typeName": ".google.protobuf.FeatureSet"
			}, {
				"name": "uninterpreted_option",
				"number": 999,
				"type": 11,
				"label": 3,
				"typeName": ".google.protobuf.UninterpretedOption"
			}],
			"extensionRange": [{
				"start": 990,
				"end": 999
			}, {
				"start": 1e3,
				"end": 536870912
			}]
		},
		{
			"name": "EnumOptions",
			"field": [
				{
					"name": "allow_alias",
					"number": 2,
					"type": 8,
					"label": 1
				},
				{
					"name": "deprecated",
					"number": 3,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "deprecated_legacy_json_field_conflicts",
					"number": 6,
					"type": 8,
					"label": 1,
					"options": { "deprecated": true }
				},
				{
					"name": "features",
					"number": 7,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet"
				},
				{
					"name": "uninterpreted_option",
					"number": 999,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.UninterpretedOption"
				}
			],
			"extensionRange": [{
				"start": 990,
				"end": 999
			}, {
				"start": 1e3,
				"end": 536870912
			}]
		},
		{
			"name": "EnumValueOptions",
			"field": [
				{
					"name": "deprecated",
					"number": 1,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "features",
					"number": 2,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet"
				},
				{
					"name": "debug_redact",
					"number": 3,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "feature_support",
					"number": 4,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.FieldOptions.FeatureSupport"
				},
				{
					"name": "uninterpreted_option",
					"number": 999,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.UninterpretedOption"
				}
			],
			"extensionRange": [{
				"start": 990,
				"end": 999
			}, {
				"start": 1e3,
				"end": 536870912
			}]
		},
		{
			"name": "ServiceOptions",
			"field": [
				{
					"name": "features",
					"number": 34,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet"
				},
				{
					"name": "deprecated",
					"number": 33,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "uninterpreted_option",
					"number": 999,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.UninterpretedOption"
				}
			],
			"extensionRange": [{
				"start": 990,
				"end": 999
			}, {
				"start": 1e3,
				"end": 536870912
			}]
		},
		{
			"name": "MethodOptions",
			"field": [
				{
					"name": "deprecated",
					"number": 33,
					"type": 8,
					"label": 1,
					"defaultValue": "false"
				},
				{
					"name": "idempotency_level",
					"number": 34,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.MethodOptions.IdempotencyLevel",
					"defaultValue": "IDEMPOTENCY_UNKNOWN"
				},
				{
					"name": "features",
					"number": 35,
					"type": 11,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet"
				},
				{
					"name": "uninterpreted_option",
					"number": 999,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.UninterpretedOption"
				}
			],
			"enumType": [{
				"name": "IdempotencyLevel",
				"value": [
					{
						"name": "IDEMPOTENCY_UNKNOWN",
						"number": 0
					},
					{
						"name": "NO_SIDE_EFFECTS",
						"number": 1
					},
					{
						"name": "IDEMPOTENT",
						"number": 2
					}
				]
			}],
			"extensionRange": [{
				"start": 990,
				"end": 999
			}, {
				"start": 1e3,
				"end": 536870912
			}]
		},
		{
			"name": "UninterpretedOption",
			"field": [
				{
					"name": "name",
					"number": 2,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.UninterpretedOption.NamePart"
				},
				{
					"name": "identifier_value",
					"number": 3,
					"type": 9,
					"label": 1
				},
				{
					"name": "positive_int_value",
					"number": 4,
					"type": 4,
					"label": 1
				},
				{
					"name": "negative_int_value",
					"number": 5,
					"type": 3,
					"label": 1
				},
				{
					"name": "double_value",
					"number": 6,
					"type": 1,
					"label": 1
				},
				{
					"name": "string_value",
					"number": 7,
					"type": 12,
					"label": 1
				},
				{
					"name": "aggregate_value",
					"number": 8,
					"type": 9,
					"label": 1
				}
			],
			"nestedType": [{
				"name": "NamePart",
				"field": [{
					"name": "name_part",
					"number": 1,
					"type": 9,
					"label": 2
				}, {
					"name": "is_extension",
					"number": 2,
					"type": 8,
					"label": 2
				}]
			}]
		},
		{
			"name": "FeatureSet",
			"field": [
				{
					"name": "field_presence",
					"number": 1,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet.FieldPresence",
					"options": {
						"retention": 1,
						"targets": [4, 1],
						"editionDefaults": [
							{
								"value": "EXPLICIT",
								"edition": 900
							},
							{
								"value": "IMPLICIT",
								"edition": 999
							},
							{
								"value": "EXPLICIT",
								"edition": 1e3
							}
						]
					}
				},
				{
					"name": "enum_type",
					"number": 2,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet.EnumType",
					"options": {
						"retention": 1,
						"targets": [6, 1],
						"editionDefaults": [{
							"value": "CLOSED",
							"edition": 900
						}, {
							"value": "OPEN",
							"edition": 999
						}]
					}
				},
				{
					"name": "repeated_field_encoding",
					"number": 3,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet.RepeatedFieldEncoding",
					"options": {
						"retention": 1,
						"targets": [4, 1],
						"editionDefaults": [{
							"value": "EXPANDED",
							"edition": 900
						}, {
							"value": "PACKED",
							"edition": 999
						}]
					}
				},
				{
					"name": "utf8_validation",
					"number": 4,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet.Utf8Validation",
					"options": {
						"retention": 1,
						"targets": [4, 1],
						"editionDefaults": [{
							"value": "NONE",
							"edition": 900
						}, {
							"value": "VERIFY",
							"edition": 999
						}]
					}
				},
				{
					"name": "message_encoding",
					"number": 5,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet.MessageEncoding",
					"options": {
						"retention": 1,
						"targets": [4, 1],
						"editionDefaults": [{
							"value": "LENGTH_PREFIXED",
							"edition": 900
						}]
					}
				},
				{
					"name": "json_format",
					"number": 6,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet.JsonFormat",
					"options": {
						"retention": 1,
						"targets": [
							3,
							6,
							1
						],
						"editionDefaults": [{
							"value": "LEGACY_BEST_EFFORT",
							"edition": 900
						}, {
							"value": "ALLOW",
							"edition": 999
						}]
					}
				},
				{
					"name": "enforce_naming_style",
					"number": 7,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet.EnforceNamingStyle",
					"options": {
						"retention": 2,
						"targets": [
							1,
							2,
							3,
							4,
							5,
							6,
							7,
							8,
							9
						],
						"editionDefaults": [
							{
								"value": "STYLE_LEGACY",
								"edition": 900
							},
							{
								"value": "STYLE2024",
								"edition": 1001
							},
							{
								"value": "STYLE2026",
								"edition": 1002
							}
						]
					}
				},
				{
					"name": "default_symbol_visibility",
					"number": 8,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet.VisibilityFeature.DefaultSymbolVisibility",
					"options": {
						"retention": 2,
						"targets": [1],
						"editionDefaults": [
							{
								"value": "EXPORT_ALL",
								"edition": 900
							},
							{
								"value": "EXPORT_TOP_LEVEL",
								"edition": 1001
							},
							{
								"value": "STRICT",
								"edition": 1002
							}
						]
					}
				},
				{
					"name": "enforce_proto_limits",
					"number": 9,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.FeatureSet.ProtoLimitsFeature.EnforceProtoLimits",
					"options": {
						"retention": 2,
						"targets": [
							6,
							3,
							4,
							5
						],
						"editionDefaults": [{
							"value": "LEGACY_NO_EXPLICIT_LIMITS",
							"edition": 900
						}, {
							"value": "PROTO_LIMITS2026",
							"edition": 1002
						}]
					}
				}
			],
			"nestedType": [{
				"name": "VisibilityFeature",
				"enumType": [{
					"name": "DefaultSymbolVisibility",
					"value": [
						{
							"name": "DEFAULT_SYMBOL_VISIBILITY_UNKNOWN",
							"number": 0
						},
						{
							"name": "EXPORT_ALL",
							"number": 1
						},
						{
							"name": "EXPORT_TOP_LEVEL",
							"number": 2
						},
						{
							"name": "LOCAL_ALL",
							"number": 3
						},
						{
							"name": "STRICT",
							"number": 4
						}
					]
				}]
			}, {
				"name": "ProtoLimitsFeature",
				"enumType": [{
					"name": "EnforceProtoLimits",
					"value": [
						{
							"name": "PROTO_LIMITS_UNKNOWN",
							"number": 0
						},
						{
							"name": "LEGACY_NO_EXPLICIT_LIMITS",
							"number": 1
						},
						{
							"name": "PROTO_LIMITS2026",
							"number": 2
						}
					]
				}]
			}],
			"enumType": [
				{
					"name": "FieldPresence",
					"value": [
						{
							"name": "FIELD_PRESENCE_UNKNOWN",
							"number": 0
						},
						{
							"name": "EXPLICIT",
							"number": 1
						},
						{
							"name": "IMPLICIT",
							"number": 2
						},
						{
							"name": "LEGACY_REQUIRED",
							"number": 3
						}
					]
				},
				{
					"name": "EnumType",
					"value": [
						{
							"name": "ENUM_TYPE_UNKNOWN",
							"number": 0
						},
						{
							"name": "OPEN",
							"number": 1
						},
						{
							"name": "CLOSED",
							"number": 2
						}
					]
				},
				{
					"name": "RepeatedFieldEncoding",
					"value": [
						{
							"name": "REPEATED_FIELD_ENCODING_UNKNOWN",
							"number": 0
						},
						{
							"name": "PACKED",
							"number": 1
						},
						{
							"name": "EXPANDED",
							"number": 2
						}
					]
				},
				{
					"name": "Utf8Validation",
					"value": [
						{
							"name": "UTF8_VALIDATION_UNKNOWN",
							"number": 0
						},
						{
							"name": "VERIFY",
							"number": 2
						},
						{
							"name": "NONE",
							"number": 3
						}
					]
				},
				{
					"name": "MessageEncoding",
					"value": [
						{
							"name": "MESSAGE_ENCODING_UNKNOWN",
							"number": 0
						},
						{
							"name": "LENGTH_PREFIXED",
							"number": 1
						},
						{
							"name": "DELIMITED",
							"number": 2
						}
					]
				},
				{
					"name": "JsonFormat",
					"value": [
						{
							"name": "JSON_FORMAT_UNKNOWN",
							"number": 0
						},
						{
							"name": "ALLOW",
							"number": 1
						},
						{
							"name": "LEGACY_BEST_EFFORT",
							"number": 2
						}
					]
				},
				{
					"name": "EnforceNamingStyle",
					"value": [
						{
							"name": "ENFORCE_NAMING_STYLE_UNKNOWN",
							"number": 0
						},
						{
							"name": "STYLE2024",
							"number": 1
						},
						{
							"name": "STYLE_LEGACY",
							"number": 2
						},
						{
							"name": "STYLE2026",
							"number": 3
						}
					]
				}
			],
			"extensionRange": [
				{
					"start": 1e3,
					"end": 9995
				},
				{
					"start": 9995,
					"end": 1e4
				},
				{
					"start": 1e4,
					"end": 10001
				}
			]
		},
		{
			"name": "FeatureSetDefaults",
			"field": [
				{
					"name": "defaults",
					"number": 1,
					"type": 11,
					"label": 3,
					"typeName": ".google.protobuf.FeatureSetDefaults.FeatureSetEditionDefault"
				},
				{
					"name": "minimum_edition",
					"number": 4,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.Edition"
				},
				{
					"name": "maximum_edition",
					"number": 5,
					"type": 14,
					"label": 1,
					"typeName": ".google.protobuf.Edition"
				}
			],
			"nestedType": [{
				"name": "FeatureSetEditionDefault",
				"field": [
					{
						"name": "edition",
						"number": 3,
						"type": 14,
						"label": 1,
						"typeName": ".google.protobuf.Edition"
					},
					{
						"name": "overridable_features",
						"number": 4,
						"type": 11,
						"label": 1,
						"typeName": ".google.protobuf.FeatureSet"
					},
					{
						"name": "fixed_features",
						"number": 5,
						"type": 11,
						"label": 1,
						"typeName": ".google.protobuf.FeatureSet"
					}
				]
			}]
		},
		{
			"name": "SourceCodeInfo",
			"field": [{
				"name": "location",
				"number": 1,
				"type": 11,
				"label": 3,
				"typeName": ".google.protobuf.SourceCodeInfo.Location"
			}],
			"nestedType": [{
				"name": "Location",
				"field": [
					{
						"name": "path",
						"number": 1,
						"type": 5,
						"label": 3,
						"options": { "packed": true }
					},
					{
						"name": "span",
						"number": 2,
						"type": 5,
						"label": 3,
						"options": { "packed": true }
					},
					{
						"name": "leading_comments",
						"number": 3,
						"type": 9,
						"label": 1
					},
					{
						"name": "trailing_comments",
						"number": 4,
						"type": 9,
						"label": 1
					},
					{
						"name": "leading_detached_comments",
						"number": 6,
						"type": 9,
						"label": 3
					}
				]
			}],
			"extensionRange": [{
				"start": 536e6,
				"end": 536000001
			}]
		},
		{
			"name": "GeneratedCodeInfo",
			"field": [{
				"name": "annotation",
				"number": 1,
				"type": 11,
				"label": 3,
				"typeName": ".google.protobuf.GeneratedCodeInfo.Annotation"
			}],
			"nestedType": [{
				"name": "Annotation",
				"field": [
					{
						"name": "path",
						"number": 1,
						"type": 5,
						"label": 3,
						"options": { "packed": true }
					},
					{
						"name": "source_file",
						"number": 2,
						"type": 9,
						"label": 1
					},
					{
						"name": "begin",
						"number": 3,
						"type": 5,
						"label": 1
					},
					{
						"name": "end",
						"number": 4,
						"type": 5,
						"label": 1
					},
					{
						"name": "semantic",
						"number": 5,
						"type": 14,
						"label": 1,
						"typeName": ".google.protobuf.GeneratedCodeInfo.Annotation.Semantic"
					}
				],
				"enumType": [{
					"name": "Semantic",
					"value": [
						{
							"name": "NONE",
							"number": 0
						},
						{
							"name": "SET",
							"number": 1
						},
						{
							"name": "ALIAS",
							"number": 2
						}
					]
				}]
			}]
		}
	],
	"enumType": [{
		"name": "Edition",
		"value": [
			{
				"name": "EDITION_UNKNOWN",
				"number": 0
			},
			{
				"name": "EDITION_LEGACY",
				"number": 900
			},
			{
				"name": "EDITION_PROTO2",
				"number": 998
			},
			{
				"name": "EDITION_PROTO3",
				"number": 999
			},
			{
				"name": "EDITION_2023",
				"number": 1e3
			},
			{
				"name": "EDITION_2024",
				"number": 1001
			},
			{
				"name": "EDITION_2026",
				"number": 1002
			},
			{
				"name": "EDITION_UNSTABLE",
				"number": 9999
			},
			{
				"name": "EDITION_1_TEST_ONLY",
				"number": 1
			},
			{
				"name": "EDITION_2_TEST_ONLY",
				"number": 2
			},
			{
				"name": "EDITION_99997_TEST_ONLY",
				"number": 99997
			},
			{
				"name": "EDITION_99998_TEST_ONLY",
				"number": 99998
			},
			{
				"name": "EDITION_99999_TEST_ONLY",
				"number": 99999
			},
			{
				"name": "EDITION_MAX",
				"number": 2147483647
			}
		]
	}, {
		"name": "SymbolVisibility",
		"value": [
			{
				"name": "VISIBILITY_UNSET",
				"number": 0
			},
			{
				"name": "VISIBILITY_LOCAL",
				"number": 1
			},
			{
				"name": "VISIBILITY_EXPORT",
				"number": 2
			}
		]
	}]
}), 1);
/**
* The verification state of the extension range.
*
* @generated from enum google.protobuf.ExtensionRangeOptions.VerificationState
*/
var ExtensionRangeOptions_VerificationState;
(function(ExtensionRangeOptions_VerificationState) {
	/**
	* All the extensions of the range must be declared.
	*
	* @generated from enum value: DECLARATION = 0;
	*/
	ExtensionRangeOptions_VerificationState[ExtensionRangeOptions_VerificationState["DECLARATION"] = 0] = "DECLARATION";
	/**
	* @generated from enum value: UNVERIFIED = 1;
	*/
	ExtensionRangeOptions_VerificationState[ExtensionRangeOptions_VerificationState["UNVERIFIED"] = 1] = "UNVERIFIED";
})(ExtensionRangeOptions_VerificationState || (ExtensionRangeOptions_VerificationState = {}));
/**
* @generated from enum google.protobuf.FieldDescriptorProto.Type
*/
var FieldDescriptorProto_Type;
(function(FieldDescriptorProto_Type) {
	/**
	* 0 is reserved for errors.
	* Order is weird for historical reasons.
	*
	* @generated from enum value: TYPE_DOUBLE = 1;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["DOUBLE"] = 1] = "DOUBLE";
	/**
	* @generated from enum value: TYPE_FLOAT = 2;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["FLOAT"] = 2] = "FLOAT";
	/**
	* Not ZigZag encoded.  Negative numbers take 10 bytes.  Use TYPE_SINT64 if
	* negative values are likely.
	*
	* @generated from enum value: TYPE_INT64 = 3;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["INT64"] = 3] = "INT64";
	/**
	* @generated from enum value: TYPE_UINT64 = 4;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["UINT64"] = 4] = "UINT64";
	/**
	* Not ZigZag encoded.  Negative numbers take 10 bytes.  Use TYPE_SINT32 if
	* negative values are likely.
	*
	* @generated from enum value: TYPE_INT32 = 5;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["INT32"] = 5] = "INT32";
	/**
	* @generated from enum value: TYPE_FIXED64 = 6;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["FIXED64"] = 6] = "FIXED64";
	/**
	* @generated from enum value: TYPE_FIXED32 = 7;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["FIXED32"] = 7] = "FIXED32";
	/**
	* @generated from enum value: TYPE_BOOL = 8;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["BOOL"] = 8] = "BOOL";
	/**
	* @generated from enum value: TYPE_STRING = 9;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["STRING"] = 9] = "STRING";
	/**
	* Tag-delimited aggregate.
	* Group type is deprecated and not supported after google.protobuf. However, Proto3
	* implementations should still be able to parse the group wire format and
	* treat group fields as unknown fields.  In Editions, the group wire format
	* can be enabled via the `message_encoding` feature.
	*
	* @generated from enum value: TYPE_GROUP = 10;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["GROUP"] = 10] = "GROUP";
	/**
	* Length-delimited aggregate.
	*
	* @generated from enum value: TYPE_MESSAGE = 11;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["MESSAGE"] = 11] = "MESSAGE";
	/**
	* New in version 2.
	*
	* @generated from enum value: TYPE_BYTES = 12;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["BYTES"] = 12] = "BYTES";
	/**
	* @generated from enum value: TYPE_UINT32 = 13;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["UINT32"] = 13] = "UINT32";
	/**
	* @generated from enum value: TYPE_ENUM = 14;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["ENUM"] = 14] = "ENUM";
	/**
	* @generated from enum value: TYPE_SFIXED32 = 15;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["SFIXED32"] = 15] = "SFIXED32";
	/**
	* @generated from enum value: TYPE_SFIXED64 = 16;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["SFIXED64"] = 16] = "SFIXED64";
	/**
	* Uses ZigZag encoding.
	*
	* @generated from enum value: TYPE_SINT32 = 17;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["SINT32"] = 17] = "SINT32";
	/**
	* Uses ZigZag encoding.
	*
	* @generated from enum value: TYPE_SINT64 = 18;
	*/
	FieldDescriptorProto_Type[FieldDescriptorProto_Type["SINT64"] = 18] = "SINT64";
})(FieldDescriptorProto_Type || (FieldDescriptorProto_Type = {}));
/**
* @generated from enum google.protobuf.FieldDescriptorProto.Label
*/
var FieldDescriptorProto_Label;
(function(FieldDescriptorProto_Label) {
	/**
	* 0 is reserved for errors
	*
	* @generated from enum value: LABEL_OPTIONAL = 1;
	*/
	FieldDescriptorProto_Label[FieldDescriptorProto_Label["OPTIONAL"] = 1] = "OPTIONAL";
	/**
	* @generated from enum value: LABEL_REPEATED = 3;
	*/
	FieldDescriptorProto_Label[FieldDescriptorProto_Label["REPEATED"] = 3] = "REPEATED";
	/**
	* The required label is only allowed in google.protobuf.  In proto3 and Editions
	* it's explicitly prohibited.  In Editions, the `field_presence` feature
	* can be used to get this behavior.
	*
	* @generated from enum value: LABEL_REQUIRED = 2;
	*/
	FieldDescriptorProto_Label[FieldDescriptorProto_Label["REQUIRED"] = 2] = "REQUIRED";
})(FieldDescriptorProto_Label || (FieldDescriptorProto_Label = {}));
/**
* Generated classes can be optimized for speed or code size.
*
* @generated from enum google.protobuf.FileOptions.OptimizeMode
*/
var FileOptions_OptimizeMode;
(function(FileOptions_OptimizeMode) {
	/**
	* Generate complete code for parsing, serialization,
	*
	* @generated from enum value: SPEED = 1;
	*/
	FileOptions_OptimizeMode[FileOptions_OptimizeMode["SPEED"] = 1] = "SPEED";
	/**
	* etc.
	*
	* Use ReflectionOps to implement these methods.
	*
	* @generated from enum value: CODE_SIZE = 2;
	*/
	FileOptions_OptimizeMode[FileOptions_OptimizeMode["CODE_SIZE"] = 2] = "CODE_SIZE";
	/**
	* Generate code using MessageLite and the lite runtime.
	*
	* @generated from enum value: LITE_RUNTIME = 3;
	*/
	FileOptions_OptimizeMode[FileOptions_OptimizeMode["LITE_RUNTIME"] = 3] = "LITE_RUNTIME";
})(FileOptions_OptimizeMode || (FileOptions_OptimizeMode = {}));
/**
* @generated from enum google.protobuf.FieldOptions.CType
*/
var FieldOptions_CType;
(function(FieldOptions_CType) {
	/**
	* Default mode.
	*
	* @generated from enum value: STRING = 0;
	*/
	FieldOptions_CType[FieldOptions_CType["STRING"] = 0] = "STRING";
	/**
	* The option [ctype=CORD] may be applied to a non-repeated field of type
	* "bytes". It indicates that in C++, the data should be stored in a Cord
	* instead of a string.  For very large strings, this may reduce memory
	* fragmentation. It may also allow better performance when parsing from a
	* Cord, or when parsing with aliasing enabled, as the parsed Cord may then
	* alias the original buffer.
	*
	* @generated from enum value: CORD = 1;
	*/
	FieldOptions_CType[FieldOptions_CType["CORD"] = 1] = "CORD";
	/**
	* @generated from enum value: STRING_PIECE = 2;
	*/
	FieldOptions_CType[FieldOptions_CType["STRING_PIECE"] = 2] = "STRING_PIECE";
})(FieldOptions_CType || (FieldOptions_CType = {}));
/**
* @generated from enum google.protobuf.FieldOptions.JSType
*/
var FieldOptions_JSType;
(function(FieldOptions_JSType) {
	/**
	* Use the default type.
	*
	* @generated from enum value: JS_NORMAL = 0;
	*/
	FieldOptions_JSType[FieldOptions_JSType["JS_NORMAL"] = 0] = "JS_NORMAL";
	/**
	* Use JavaScript strings.
	*
	* @generated from enum value: JS_STRING = 1;
	*/
	FieldOptions_JSType[FieldOptions_JSType["JS_STRING"] = 1] = "JS_STRING";
	/**
	* Use JavaScript numbers.
	*
	* @generated from enum value: JS_NUMBER = 2;
	*/
	FieldOptions_JSType[FieldOptions_JSType["JS_NUMBER"] = 2] = "JS_NUMBER";
})(FieldOptions_JSType || (FieldOptions_JSType = {}));
/**
* If set to RETENTION_SOURCE, the option will be omitted from the binary.
*
* @generated from enum google.protobuf.FieldOptions.OptionRetention
*/
var FieldOptions_OptionRetention;
(function(FieldOptions_OptionRetention) {
	/**
	* @generated from enum value: RETENTION_UNKNOWN = 0;
	*/
	FieldOptions_OptionRetention[FieldOptions_OptionRetention["RETENTION_UNKNOWN"] = 0] = "RETENTION_UNKNOWN";
	/**
	* @generated from enum value: RETENTION_RUNTIME = 1;
	*/
	FieldOptions_OptionRetention[FieldOptions_OptionRetention["RETENTION_RUNTIME"] = 1] = "RETENTION_RUNTIME";
	/**
	* @generated from enum value: RETENTION_SOURCE = 2;
	*/
	FieldOptions_OptionRetention[FieldOptions_OptionRetention["RETENTION_SOURCE"] = 2] = "RETENTION_SOURCE";
})(FieldOptions_OptionRetention || (FieldOptions_OptionRetention = {}));
/**
* This indicates the types of entities that the field may apply to when used
* as an option. If it is unset, then the field may be freely used as an
* option on any kind of entity.
*
* @generated from enum google.protobuf.FieldOptions.OptionTargetType
*/
var FieldOptions_OptionTargetType;
(function(FieldOptions_OptionTargetType) {
	/**
	* @generated from enum value: TARGET_TYPE_UNKNOWN = 0;
	*/
	FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_UNKNOWN"] = 0] = "TARGET_TYPE_UNKNOWN";
	/**
	* @generated from enum value: TARGET_TYPE_FILE = 1;
	*/
	FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_FILE"] = 1] = "TARGET_TYPE_FILE";
	/**
	* @generated from enum value: TARGET_TYPE_EXTENSION_RANGE = 2;
	*/
	FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_EXTENSION_RANGE"] = 2] = "TARGET_TYPE_EXTENSION_RANGE";
	/**
	* @generated from enum value: TARGET_TYPE_MESSAGE = 3;
	*/
	FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_MESSAGE"] = 3] = "TARGET_TYPE_MESSAGE";
	/**
	* @generated from enum value: TARGET_TYPE_FIELD = 4;
	*/
	FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_FIELD"] = 4] = "TARGET_TYPE_FIELD";
	/**
	* @generated from enum value: TARGET_TYPE_ONEOF = 5;
	*/
	FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_ONEOF"] = 5] = "TARGET_TYPE_ONEOF";
	/**
	* @generated from enum value: TARGET_TYPE_ENUM = 6;
	*/
	FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_ENUM"] = 6] = "TARGET_TYPE_ENUM";
	/**
	* @generated from enum value: TARGET_TYPE_ENUM_ENTRY = 7;
	*/
	FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_ENUM_ENTRY"] = 7] = "TARGET_TYPE_ENUM_ENTRY";
	/**
	* @generated from enum value: TARGET_TYPE_SERVICE = 8;
	*/
	FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_SERVICE"] = 8] = "TARGET_TYPE_SERVICE";
	/**
	* @generated from enum value: TARGET_TYPE_METHOD = 9;
	*/
	FieldOptions_OptionTargetType[FieldOptions_OptionTargetType["TARGET_TYPE_METHOD"] = 9] = "TARGET_TYPE_METHOD";
})(FieldOptions_OptionTargetType || (FieldOptions_OptionTargetType = {}));
/**
* Is this method side-effect-free (or safe in HTTP parlance), or idempotent,
* or neither? HTTP based RPC implementation may choose GET verb for safe
* methods, and PUT verb for idempotent methods instead of the default POST.
*
* @generated from enum google.protobuf.MethodOptions.IdempotencyLevel
*/
var MethodOptions_IdempotencyLevel;
(function(MethodOptions_IdempotencyLevel) {
	/**
	* @generated from enum value: IDEMPOTENCY_UNKNOWN = 0;
	*/
	MethodOptions_IdempotencyLevel[MethodOptions_IdempotencyLevel["IDEMPOTENCY_UNKNOWN"] = 0] = "IDEMPOTENCY_UNKNOWN";
	/**
	* implies idempotent
	*
	* @generated from enum value: NO_SIDE_EFFECTS = 1;
	*/
	MethodOptions_IdempotencyLevel[MethodOptions_IdempotencyLevel["NO_SIDE_EFFECTS"] = 1] = "NO_SIDE_EFFECTS";
	/**
	* idempotent, but may have side effects
	*
	* @generated from enum value: IDEMPOTENT = 2;
	*/
	MethodOptions_IdempotencyLevel[MethodOptions_IdempotencyLevel["IDEMPOTENT"] = 2] = "IDEMPOTENT";
})(MethodOptions_IdempotencyLevel || (MethodOptions_IdempotencyLevel = {}));
/**
* @generated from enum google.protobuf.FeatureSet.VisibilityFeature.DefaultSymbolVisibility
*/
var FeatureSet_VisibilityFeature_DefaultSymbolVisibility;
(function(FeatureSet_VisibilityFeature_DefaultSymbolVisibility) {
	/**
	* @generated from enum value: DEFAULT_SYMBOL_VISIBILITY_UNKNOWN = 0;
	*/
	FeatureSet_VisibilityFeature_DefaultSymbolVisibility[FeatureSet_VisibilityFeature_DefaultSymbolVisibility["DEFAULT_SYMBOL_VISIBILITY_UNKNOWN"] = 0] = "DEFAULT_SYMBOL_VISIBILITY_UNKNOWN";
	/**
	* Default pre-EDITION_2024, all UNSET visibility are export.
	*
	* @generated from enum value: EXPORT_ALL = 1;
	*/
	FeatureSet_VisibilityFeature_DefaultSymbolVisibility[FeatureSet_VisibilityFeature_DefaultSymbolVisibility["EXPORT_ALL"] = 1] = "EXPORT_ALL";
	/**
	* All top-level symbols default to export, nested default to local.
	*
	* @generated from enum value: EXPORT_TOP_LEVEL = 2;
	*/
	FeatureSet_VisibilityFeature_DefaultSymbolVisibility[FeatureSet_VisibilityFeature_DefaultSymbolVisibility["EXPORT_TOP_LEVEL"] = 2] = "EXPORT_TOP_LEVEL";
	/**
	* All symbols default to local.
	*
	* @generated from enum value: LOCAL_ALL = 3;
	*/
	FeatureSet_VisibilityFeature_DefaultSymbolVisibility[FeatureSet_VisibilityFeature_DefaultSymbolVisibility["LOCAL_ALL"] = 3] = "LOCAL_ALL";
	/**
	* All symbols local by default. Nested types cannot be exported.
	* With special case caveat for message { enum {} reserved 1 to max; }
	* This is the recommended setting for new protos.
	*
	* @generated from enum value: STRICT = 4;
	*/
	FeatureSet_VisibilityFeature_DefaultSymbolVisibility[FeatureSet_VisibilityFeature_DefaultSymbolVisibility["STRICT"] = 4] = "STRICT";
})(FeatureSet_VisibilityFeature_DefaultSymbolVisibility || (FeatureSet_VisibilityFeature_DefaultSymbolVisibility = {}));
/**
* @generated from enum google.protobuf.FeatureSet.ProtoLimitsFeature.EnforceProtoLimits
*/
var FeatureSet_ProtoLimitsFeature_EnforceProtoLimits;
(function(FeatureSet_ProtoLimitsFeature_EnforceProtoLimits) {
	/**
	* @generated from enum value: PROTO_LIMITS_UNKNOWN = 0;
	*/
	FeatureSet_ProtoLimitsFeature_EnforceProtoLimits[FeatureSet_ProtoLimitsFeature_EnforceProtoLimits["PROTO_LIMITS_UNKNOWN"] = 0] = "PROTO_LIMITS_UNKNOWN";
	/**
	* Default pre-EDITION_2026: there are no limit enforcement at the protoc
	* level. Practical limits still exist, but they will tend to fail while
	* compiling protoc-generated code, and these limits tend to be language
	* or toolchain specific.
	*
	* @generated from enum value: LEGACY_NO_EXPLICIT_LIMITS = 1;
	*/
	FeatureSet_ProtoLimitsFeature_EnforceProtoLimits[FeatureSet_ProtoLimitsFeature_EnforceProtoLimits["LEGACY_NO_EXPLICIT_LIMITS"] = 1] = "LEGACY_NO_EXPLICIT_LIMITS";
	/**
	* A set of limits enforced by Edition 2026 by default. For a detailed
	* list of all the limits please consult the Edition 2026 documentation.
	*
	* @generated from enum value: PROTO_LIMITS2026 = 2;
	*/
	FeatureSet_ProtoLimitsFeature_EnforceProtoLimits[FeatureSet_ProtoLimitsFeature_EnforceProtoLimits["PROTO_LIMITS2026"] = 2] = "PROTO_LIMITS2026";
})(FeatureSet_ProtoLimitsFeature_EnforceProtoLimits || (FeatureSet_ProtoLimitsFeature_EnforceProtoLimits = {}));
/**
* @generated from enum google.protobuf.FeatureSet.FieldPresence
*/
var FeatureSet_FieldPresence;
(function(FeatureSet_FieldPresence) {
	/**
	* @generated from enum value: FIELD_PRESENCE_UNKNOWN = 0;
	*/
	FeatureSet_FieldPresence[FeatureSet_FieldPresence["FIELD_PRESENCE_UNKNOWN"] = 0] = "FIELD_PRESENCE_UNKNOWN";
	/**
	* @generated from enum value: EXPLICIT = 1;
	*/
	FeatureSet_FieldPresence[FeatureSet_FieldPresence["EXPLICIT"] = 1] = "EXPLICIT";
	/**
	* @generated from enum value: IMPLICIT = 2;
	*/
	FeatureSet_FieldPresence[FeatureSet_FieldPresence["IMPLICIT"] = 2] = "IMPLICIT";
	/**
	* @generated from enum value: LEGACY_REQUIRED = 3;
	*/
	FeatureSet_FieldPresence[FeatureSet_FieldPresence["LEGACY_REQUIRED"] = 3] = "LEGACY_REQUIRED";
})(FeatureSet_FieldPresence || (FeatureSet_FieldPresence = {}));
/**
* @generated from enum google.protobuf.FeatureSet.EnumType
*/
var FeatureSet_EnumType;
(function(FeatureSet_EnumType) {
	/**
	* @generated from enum value: ENUM_TYPE_UNKNOWN = 0;
	*/
	FeatureSet_EnumType[FeatureSet_EnumType["ENUM_TYPE_UNKNOWN"] = 0] = "ENUM_TYPE_UNKNOWN";
	/**
	* @generated from enum value: OPEN = 1;
	*/
	FeatureSet_EnumType[FeatureSet_EnumType["OPEN"] = 1] = "OPEN";
	/**
	* @generated from enum value: CLOSED = 2;
	*/
	FeatureSet_EnumType[FeatureSet_EnumType["CLOSED"] = 2] = "CLOSED";
})(FeatureSet_EnumType || (FeatureSet_EnumType = {}));
/**
* @generated from enum google.protobuf.FeatureSet.RepeatedFieldEncoding
*/
var FeatureSet_RepeatedFieldEncoding;
(function(FeatureSet_RepeatedFieldEncoding) {
	/**
	* @generated from enum value: REPEATED_FIELD_ENCODING_UNKNOWN = 0;
	*/
	FeatureSet_RepeatedFieldEncoding[FeatureSet_RepeatedFieldEncoding["REPEATED_FIELD_ENCODING_UNKNOWN"] = 0] = "REPEATED_FIELD_ENCODING_UNKNOWN";
	/**
	* @generated from enum value: PACKED = 1;
	*/
	FeatureSet_RepeatedFieldEncoding[FeatureSet_RepeatedFieldEncoding["PACKED"] = 1] = "PACKED";
	/**
	* @generated from enum value: EXPANDED = 2;
	*/
	FeatureSet_RepeatedFieldEncoding[FeatureSet_RepeatedFieldEncoding["EXPANDED"] = 2] = "EXPANDED";
})(FeatureSet_RepeatedFieldEncoding || (FeatureSet_RepeatedFieldEncoding = {}));
/**
* @generated from enum google.protobuf.FeatureSet.Utf8Validation
*/
var FeatureSet_Utf8Validation;
(function(FeatureSet_Utf8Validation) {
	/**
	* @generated from enum value: UTF8_VALIDATION_UNKNOWN = 0;
	*/
	FeatureSet_Utf8Validation[FeatureSet_Utf8Validation["UTF8_VALIDATION_UNKNOWN"] = 0] = "UTF8_VALIDATION_UNKNOWN";
	/**
	* @generated from enum value: VERIFY = 2;
	*/
	FeatureSet_Utf8Validation[FeatureSet_Utf8Validation["VERIFY"] = 2] = "VERIFY";
	/**
	* @generated from enum value: NONE = 3;
	*/
	FeatureSet_Utf8Validation[FeatureSet_Utf8Validation["NONE"] = 3] = "NONE";
})(FeatureSet_Utf8Validation || (FeatureSet_Utf8Validation = {}));
/**
* @generated from enum google.protobuf.FeatureSet.MessageEncoding
*/
var FeatureSet_MessageEncoding;
(function(FeatureSet_MessageEncoding) {
	/**
	* @generated from enum value: MESSAGE_ENCODING_UNKNOWN = 0;
	*/
	FeatureSet_MessageEncoding[FeatureSet_MessageEncoding["MESSAGE_ENCODING_UNKNOWN"] = 0] = "MESSAGE_ENCODING_UNKNOWN";
	/**
	* @generated from enum value: LENGTH_PREFIXED = 1;
	*/
	FeatureSet_MessageEncoding[FeatureSet_MessageEncoding["LENGTH_PREFIXED"] = 1] = "LENGTH_PREFIXED";
	/**
	* @generated from enum value: DELIMITED = 2;
	*/
	FeatureSet_MessageEncoding[FeatureSet_MessageEncoding["DELIMITED"] = 2] = "DELIMITED";
})(FeatureSet_MessageEncoding || (FeatureSet_MessageEncoding = {}));
/**
* @generated from enum google.protobuf.FeatureSet.JsonFormat
*/
var FeatureSet_JsonFormat;
(function(FeatureSet_JsonFormat) {
	/**
	* @generated from enum value: JSON_FORMAT_UNKNOWN = 0;
	*/
	FeatureSet_JsonFormat[FeatureSet_JsonFormat["JSON_FORMAT_UNKNOWN"] = 0] = "JSON_FORMAT_UNKNOWN";
	/**
	* @generated from enum value: ALLOW = 1;
	*/
	FeatureSet_JsonFormat[FeatureSet_JsonFormat["ALLOW"] = 1] = "ALLOW";
	/**
	* @generated from enum value: LEGACY_BEST_EFFORT = 2;
	*/
	FeatureSet_JsonFormat[FeatureSet_JsonFormat["LEGACY_BEST_EFFORT"] = 2] = "LEGACY_BEST_EFFORT";
})(FeatureSet_JsonFormat || (FeatureSet_JsonFormat = {}));
/**
* @generated from enum google.protobuf.FeatureSet.EnforceNamingStyle
*/
var FeatureSet_EnforceNamingStyle;
(function(FeatureSet_EnforceNamingStyle) {
	/**
	* @generated from enum value: ENFORCE_NAMING_STYLE_UNKNOWN = 0;
	*/
	FeatureSet_EnforceNamingStyle[FeatureSet_EnforceNamingStyle["ENFORCE_NAMING_STYLE_UNKNOWN"] = 0] = "ENFORCE_NAMING_STYLE_UNKNOWN";
	/**
	* @generated from enum value: STYLE2024 = 1;
	*/
	FeatureSet_EnforceNamingStyle[FeatureSet_EnforceNamingStyle["STYLE2024"] = 1] = "STYLE2024";
	/**
	* @generated from enum value: STYLE_LEGACY = 2;
	*/
	FeatureSet_EnforceNamingStyle[FeatureSet_EnforceNamingStyle["STYLE_LEGACY"] = 2] = "STYLE_LEGACY";
	/**
	* @generated from enum value: STYLE2026 = 3;
	*/
	FeatureSet_EnforceNamingStyle[FeatureSet_EnforceNamingStyle["STYLE2026"] = 3] = "STYLE2026";
})(FeatureSet_EnforceNamingStyle || (FeatureSet_EnforceNamingStyle = {}));
/**
* Represents the identified object's effect on the element in the original
* .proto file.
*
* @generated from enum google.protobuf.GeneratedCodeInfo.Annotation.Semantic
*/
var GeneratedCodeInfo_Annotation_Semantic;
(function(GeneratedCodeInfo_Annotation_Semantic) {
	/**
	* There is no effect or the effect is indescribable.
	*
	* @generated from enum value: NONE = 0;
	*/
	GeneratedCodeInfo_Annotation_Semantic[GeneratedCodeInfo_Annotation_Semantic["NONE"] = 0] = "NONE";
	/**
	* The element is set or otherwise mutated.
	*
	* @generated from enum value: SET = 1;
	*/
	GeneratedCodeInfo_Annotation_Semantic[GeneratedCodeInfo_Annotation_Semantic["SET"] = 1] = "SET";
	/**
	* An alias to the element is returned.
	*
	* @generated from enum value: ALIAS = 2;
	*/
	GeneratedCodeInfo_Annotation_Semantic[GeneratedCodeInfo_Annotation_Semantic["ALIAS"] = 2] = "ALIAS";
})(GeneratedCodeInfo_Annotation_Semantic || (GeneratedCodeInfo_Annotation_Semantic = {}));
/**
* The full set of known editions.
*
* @generated from enum google.protobuf.Edition
*/
var Edition;
(function(Edition) {
	/**
	* A placeholder for an unknown edition value.
	*
	* @generated from enum value: EDITION_UNKNOWN = 0;
	*/
	Edition[Edition["EDITION_UNKNOWN"] = 0] = "EDITION_UNKNOWN";
	/**
	* A placeholder edition for specifying default behaviors *before* a feature
	* was first introduced.  This is effectively an "infinite past".
	*
	* @generated from enum value: EDITION_LEGACY = 900;
	*/
	Edition[Edition["EDITION_LEGACY"] = 900] = "EDITION_LEGACY";
	/**
	* Legacy syntax "editions".  These pre-date editions, but behave much like
	* distinct editions.  These can't be used to specify the edition of proto
	* files, but feature definitions must supply proto2/proto3 defaults for
	* backwards compatibility.
	*
	* @generated from enum value: EDITION_PROTO2 = 998;
	*/
	Edition[Edition["EDITION_PROTO2"] = 998] = "EDITION_PROTO2";
	/**
	* @generated from enum value: EDITION_PROTO3 = 999;
	*/
	Edition[Edition["EDITION_PROTO3"] = 999] = "EDITION_PROTO3";
	/**
	* Editions that have been released.  The specific values are arbitrary and
	* should not be depended on, but they will always be time-ordered for easy
	* comparison.
	*
	* @generated from enum value: EDITION_2023 = 1000;
	*/
	Edition[Edition["EDITION_2023"] = 1e3] = "EDITION_2023";
	/**
	* @generated from enum value: EDITION_2024 = 1001;
	*/
	Edition[Edition["EDITION_2024"] = 1001] = "EDITION_2024";
	/**
	* @generated from enum value: EDITION_2026 = 1002;
	*/
	Edition[Edition["EDITION_2026"] = 1002] = "EDITION_2026";
	/**
	* A placeholder edition for developing and testing unscheduled features.
	*
	* @generated from enum value: EDITION_UNSTABLE = 9999;
	*/
	Edition[Edition["EDITION_UNSTABLE"] = 9999] = "EDITION_UNSTABLE";
	/**
	* Placeholder editions for testing feature resolution.  These should not be
	* used or relied on outside of tests.
	*
	* @generated from enum value: EDITION_1_TEST_ONLY = 1;
	*/
	Edition[Edition["EDITION_1_TEST_ONLY"] = 1] = "EDITION_1_TEST_ONLY";
	/**
	* @generated from enum value: EDITION_2_TEST_ONLY = 2;
	*/
	Edition[Edition["EDITION_2_TEST_ONLY"] = 2] = "EDITION_2_TEST_ONLY";
	/**
	* @generated from enum value: EDITION_99997_TEST_ONLY = 99997;
	*/
	Edition[Edition["EDITION_99997_TEST_ONLY"] = 99997] = "EDITION_99997_TEST_ONLY";
	/**
	* @generated from enum value: EDITION_99998_TEST_ONLY = 99998;
	*/
	Edition[Edition["EDITION_99998_TEST_ONLY"] = 99998] = "EDITION_99998_TEST_ONLY";
	/**
	* @generated from enum value: EDITION_99999_TEST_ONLY = 99999;
	*/
	Edition[Edition["EDITION_99999_TEST_ONLY"] = 99999] = "EDITION_99999_TEST_ONLY";
	/**
	* Placeholder for specifying unbounded edition support.  This should only
	* ever be used by plugins that can expect to never require any changes to
	* support a new edition.
	*
	* @generated from enum value: EDITION_MAX = 2147483647;
	*/
	Edition[Edition["EDITION_MAX"] = 2147483647] = "EDITION_MAX";
})(Edition || (Edition = {}));
/**
* Describes the 'visibility' of a symbol with respect to the proto import
* system. Symbols can only be imported when the visibility rules do not prevent
* it (ex: local symbols cannot be imported).  Visibility modifiers can only set
* on `message` and `enum` as they are the only types available to be referenced
* from other files.
*
* @generated from enum google.protobuf.SymbolVisibility
*/
var SymbolVisibility;
(function(SymbolVisibility) {
	/**
	* @generated from enum value: VISIBILITY_UNSET = 0;
	*/
	SymbolVisibility[SymbolVisibility["VISIBILITY_UNSET"] = 0] = "VISIBILITY_UNSET";
	/**
	* @generated from enum value: VISIBILITY_LOCAL = 1;
	*/
	SymbolVisibility[SymbolVisibility["VISIBILITY_LOCAL"] = 1] = "VISIBILITY_LOCAL";
	/**
	* @generated from enum value: VISIBILITY_EXPORT = 2;
	*/
	SymbolVisibility[SymbolVisibility["VISIBILITY_EXPORT"] = 2] = "VISIBILITY_EXPORT";
})(SymbolVisibility || (SymbolVisibility = {}));
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/from-binary.js
/**
* @private Only exported for getExtension()
*/
function makeReadContext$1(options) {
	return Object.assign(Object.assign({
		readUnknownFields: true,
		recursionLimit: 100
	}, options), { depth: 0 });
}
/**
* Parse serialized binary data.
*/
function fromBinary(schema, bytes, options) {
	const message = create$1(schema);
	compiledReader$1(schema).read(message, new BinaryReader(bytes), makeReadContext$1(options), bytes.byteLength);
	return message;
}
const compiledReaders$1 = /* @__PURE__ */ new WeakMap();
/**
* Return the compiled decoder for a message, compiling it on first use.
*/
function compiledReader$1(desc) {
	let compiled = compiledReaders$1.get(desc);
	if (compiled === void 0) compiled = compileMessage$3(desc);
	return compiled;
}
function compileMessage$3(desc) {
	const descString = String(desc);
	const fieldReaders = /* @__PURE__ */ new Map();
	const compiled = {
		read: compileMessageReader(descString, fieldReaders),
		readGroup: compileGroupReader(descString, fieldReaders)
	};
	compiledReaders$1.set(desc, compiled);
	for (const field of desc.fields) fieldReaders.set(field.number, compileFieldReader$1(field));
	return compiled;
}
/**
* Create a decoder for a length-prefixed message body, dispatching wire
* records to the compiled field decoders by field number.
*/
function compileMessageReader(descString, fieldReaders) {
	return (message, reader, ctx, length) => {
		var _a;
		if (++ctx.depth > ctx.recursionLimit) throw new Error(`cannot decode ${descString} from binary: maximum recursion depth of ${ctx.recursionLimit} reached`);
		const end = reader.pos + length;
		const unknownFields = (_a = message.$unknown) !== null && _a !== void 0 ? _a : [];
		while (reader.pos < end) {
			const [fieldNo, wireType] = reader.tag();
			const fieldReader = fieldReaders.get(fieldNo);
			if (fieldReader === void 0) {
				const data = reader.skip(wireType, fieldNo, ctx.recursionLimit - ctx.depth);
				if (ctx.readUnknownFields) unknownFields.push({
					no: fieldNo,
					wireType,
					data
				});
				continue;
			}
			fieldReader(message, reader, ctx, wireType);
		}
		if (unknownFields.length > 0) message.$unknown = unknownFields;
		ctx.depth--;
	};
}
/**
* Create a decoder for a message with the delimited encoding (group),
* reading until the EndGroup tag, like compileMessageReader.
*/
function compileGroupReader(descString, fieldReaders) {
	return (message, reader, ctx, fieldNo) => {
		var _a;
		if (++ctx.depth > ctx.recursionLimit) throw new Error(`cannot decode ${descString} from binary: maximum recursion depth of ${ctx.recursionLimit} reached`);
		let recordFieldNo;
		let wireType;
		const unknownFields = (_a = message.$unknown) !== null && _a !== void 0 ? _a : [];
		while (reader.pos < reader.len) {
			[recordFieldNo, wireType] = reader.tag();
			if (wireType == WireType.EndGroup) break;
			const fieldReader = fieldReaders.get(recordFieldNo);
			if (fieldReader === void 0) {
				const data = reader.skip(wireType, recordFieldNo, ctx.recursionLimit - ctx.depth);
				if (ctx.readUnknownFields) unknownFields.push({
					no: recordFieldNo,
					wireType,
					data
				});
				continue;
			}
			fieldReader(message, reader, ctx, wireType);
		}
		if (wireType != WireType.EndGroup || recordFieldNo !== fieldNo) throw new Error("invalid end group tag");
		if (unknownFields.length > 0) message.$unknown = unknownFields;
		ctx.depth--;
	};
}
/**
* @private Only exported for getExtension()
*/
function readField(message, reader, field, wireType, ctx) {
	compileFieldReader$1(field)(message[unsafeLocal], reader, ctx, wireType);
}
function compileFieldReader$1(field) {
	switch (field.fieldKind) {
		case "scalar": return compileScalarFieldReader$1(field);
		case "enum": return compileEnumFieldReader$1(field);
		case "message": return compileMessageFieldReader$1(field);
		case "list": return compileListFieldReader$1(field);
		case "map": return compileMapFieldReader$1(field);
	}
}
function compileScalarFieldReader$1(field) {
	const readScalar = compileScalarReader(field.scalar, field.utf8Validation, field.longAsString);
	const localName = field.localName;
	if (field.oneof) {
		const oneofLocalName = field.oneof.localName;
		return (message, reader) => {
			message[oneofLocalName] = {
				case: localName,
				value: readScalar(reader)
			};
		};
	}
	return (message, reader) => {
		message[localName] = readScalar(reader);
	};
}
function compileEnumFieldReader$1(field) {
	var _a;
	const localName = field.localName;
	const oneofLocalName = (_a = field.oneof) === null || _a === void 0 ? void 0 : _a.localName;
	if (field.enum.open) {
		if (oneofLocalName !== void 0) return (message, reader) => {
			message[oneofLocalName] = {
				case: localName,
				value: reader.int32()
			};
		};
		return (message, reader) => {
			message[localName] = reader.int32();
		};
	}
	const values = field.enum.values;
	const fieldNo = field.number;
	return (message, reader, ctx, wireType) => {
		var _a;
		const val = reader.int32();
		if (values.some((v) => v.number === val)) {
			if (oneofLocalName !== void 0) message[oneofLocalName] = {
				case: localName,
				value: val
			};
			else message[localName] = val;
		} else if (ctx.readUnknownFields) {
			const bytes = [];
			varint32write(val, bytes);
			const unknownFields = (_a = message.$unknown) !== null && _a !== void 0 ? _a : [];
			unknownFields.push({
				no: fieldNo,
				wireType,
				data: new Uint8Array(bytes)
			});
			message.$unknown = unknownFields;
		}
	};
}
function compileMessageFieldReader$1(field) {
	const localName = field.localName;
	const { toMessage, toLocal } = localMessageMapper(field);
	const readChild = compileChildReader(field);
	if (field.oneof) {
		const oneofLocalName = field.oneof.localName;
		return (message, reader, ctx) => {
			const oneof = message[oneofLocalName];
			const child = toMessage(oneof.case === localName ? oneof.value : void 0);
			readChild(child, reader, ctx);
			message[oneofLocalName] = {
				case: localName,
				value: toLocal(child)
			};
		};
	}
	return (message, reader, ctx) => {
		const child = toMessage(message[localName]);
		readChild(child, reader, ctx);
		message[localName] = toLocal(child);
	};
}
/**
* Compile a decoder for the wire format of a message field, honoring the
* delimited encoding of the field.
*/
function compileChildReader(field) {
	const compiledChild = compiledReader$1(field.message);
	if (field.delimitedEncoding) {
		const fieldNo = field.number;
		return (child, reader, ctx) => compiledChild.readGroup(child, reader, ctx, fieldNo);
	}
	return (child, reader, ctx) => compiledChild.read(child, reader, ctx, reader.uint32());
}
function compileListFieldReader$1(field) {
	const localName = field.localName;
	if (field.listKind == "message") {
		const { toMessage, toLocal } = localMessageMapper(field);
		const readChild = compileChildReader(field);
		return (message, reader, ctx) => {
			const child = toMessage(void 0);
			readChild(child, reader, ctx);
			message[localName].push(toLocal(child));
		};
	}
	const scalarType = field.listKind == "enum" ? ScalarType.INT32 : field.scalar;
	const longAsString = field.listKind == "scalar" ? field.longAsString : false;
	const readScalar = compileScalarReader(scalarType, field.utf8Validation, longAsString);
	const packedPossible = scalarType != ScalarType.STRING && scalarType != ScalarType.BYTES;
	return (message, reader, ctx, wireType) => {
		const items = message[localName];
		if (wireType == WireType.LengthDelimited && packedPossible) {
			const end = reader.uint32() + reader.pos;
			while (reader.pos < end) items.push(readScalar(reader));
		} else items.push(readScalar(reader));
	};
}
function compileMapFieldReader$1(field) {
	const localName = field.localName;
	const readKey = compileScalarReader(field.mapKey, field.utf8Validation, false);
	const keyZero = scalarZeroValue(field.mapKey, false);
	let readValue;
	let valueDefault;
	switch (field.mapKind) {
		case "scalar": {
			const scalar = field.scalar;
			const readScalar = compileScalarReader(scalar, field.utf8Validation, false);
			readValue = (reader) => readScalar(reader);
			if (scalar == ScalarType.BYTES) valueDefault = () => /* @__PURE__ */ new Uint8Array(0);
			else {
				const zero = scalarZeroValue(scalar, false);
				valueDefault = () => zero;
			}
			break;
		}
		case "enum": {
			const zero = field.enum.values[0].number;
			readValue = (reader) => reader.int32();
			valueDefault = () => zero;
			break;
		}
		case "message": {
			const { toMessage, toLocal } = localMessageMapper(field);
			const readChild = compiledReader$1(field.message).read;
			readValue = (reader, ctx, existing) => {
				const child = toMessage(existing);
				readChild(child, reader, ctx, reader.uint32());
				return toLocal(child);
			};
			valueDefault = () => toLocal(toMessage(void 0));
			break;
		}
	}
	return (message, reader, ctx) => {
		const record = message[localName];
		let key;
		let val;
		const len = reader.uint32();
		const end = reader.pos + len;
		while (reader.pos < end) {
			const [fieldNo] = reader.tag();
			switch (fieldNo) {
				case 1:
					key = readKey(reader);
					break;
				case 2: val = readValue(reader, ctx, val);
			}
		}
		if (key === void 0) key = keyZero;
		if (val === void 0) val = valueDefault();
		setOwn(record, key, val);
	};
}
/**
* Returns a reader for a scalar value. For 64-bit integers, BinaryReader
* already returns the local representation (bigint or string), so, unlike in
* the reflection layer, no validation is needed here.
*/
function compileScalarReader(type, utf8Validation, longAsString) {
	switch (type) {
		case ScalarType.STRING: return (reader) => reader.string(utf8Validation);
		case ScalarType.BOOL: return (reader) => reader.bool();
		case ScalarType.DOUBLE: return (reader) => reader.double();
		case ScalarType.FLOAT: return (reader) => reader.float();
		case ScalarType.INT32: return (reader) => reader.int32();
		case ScalarType.INT64:
			if (longAsString) return (reader) => String(reader.int64());
			return (reader) => reader.int64();
		case ScalarType.UINT64:
			if (longAsString) return (reader) => String(reader.uint64());
			return (reader) => reader.uint64();
		case ScalarType.FIXED64:
			if (longAsString) return (reader) => String(reader.fixed64());
			return (reader) => reader.fixed64();
		case ScalarType.BYTES: return (reader) => reader.bytes();
		case ScalarType.FIXED32: return (reader) => reader.fixed32();
		case ScalarType.SFIXED32: return (reader) => reader.sfixed32();
		case ScalarType.SFIXED64:
			if (longAsString) return (reader) => String(reader.sfixed64());
			return (reader) => reader.sfixed64();
		case ScalarType.SINT64:
			if (longAsString) return (reader) => String(reader.sint64());
			return (reader) => reader.sint64();
		case ScalarType.UINT32: return (reader) => reader.uint32();
		case ScalarType.SINT32: return (reader) => reader.sint32();
	}
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/file.js
/**
* Hydrate a file descriptor.
*
* @private
*/
function fileDesc(b64, imports) {
	var _a;
	const root = fromBinary(FileDescriptorProtoSchema, base64Decode(b64));
	root.messageType.forEach(restoreJsonNames);
	root.dependency = (_a = imports === null || imports === void 0 ? void 0 : imports.map((f) => f.proto.name)) !== null && _a !== void 0 ? _a : [];
	return createFileRegistry(root, (protoFileName) => imports === null || imports === void 0 ? void 0 : imports.find((f) => f.proto.name === protoFileName)).getFile(root.name);
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/google/protobuf/timestamp_pb.js
/**
* Describes the file google/protobuf/timestamp.proto.
*/
const file_google_protobuf_timestamp = /*@__PURE__*/ fileDesc("Ch9nb29nbGUvcHJvdG9idWYvdGltZXN0YW1wLnByb3RvEg9nb29nbGUucHJvdG9idWYiKwoJVGltZXN0YW1wEg8KB3NlY29uZHMYASABKAMSDQoFbmFub3MYAiABKAVChQEKE2NvbS5nb29nbGUucHJvdG9idWZCDlRpbWVzdGFtcFByb3RvUAFaMmdvb2dsZS5nb2xhbmcub3JnL3Byb3RvYnVmL3R5cGVzL2tub3duL3RpbWVzdGFtcHBi+AEBogIDR1BCqgIeR29vZ2xlLlByb3RvYnVmLldlbGxLbm93blR5cGVzYgZwcm90bzM");
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/google/protobuf/duration_pb.js
/**
* Describes the file google/protobuf/duration.proto.
*/
const file_google_protobuf_duration = /*@__PURE__*/ fileDesc("Ch5nb29nbGUvcHJvdG9idWYvZHVyYXRpb24ucHJvdG8SD2dvb2dsZS5wcm90b2J1ZiIqCghEdXJhdGlvbhIPCgdzZWNvbmRzGAEgASgDEg0KBW5hbm9zGAIgASgFQoMBChNjb20uZ29vZ2xlLnByb3RvYnVmQg1EdXJhdGlvblByb3RvUAFaMWdvb2dsZS5nb2xhbmcub3JnL3Byb3RvYnVmL3R5cGVzL2tub3duL2R1cmF0aW9ucGL4AQGiAgNHUEKqAh5Hb29nbGUuUHJvdG9idWYuV2VsbEtub3duVHlwZXNiBnByb3RvMw");
/**
* Describes the message google.protobuf.Any.
* Use `create(AnySchema)` to create a new message.
*/
const AnySchema = /*@__PURE__*/ messageDesc(/* @__PURE__ */ fileDesc("Chlnb29nbGUvcHJvdG9idWYvYW55LnByb3RvEg9nb29nbGUucHJvdG9idWYiJgoDQW55EhAKCHR5cGVfdXJsGAEgASgJEg0KBXZhbHVlGAIgASgMQnYKE2NvbS5nb29nbGUucHJvdG9idWZCCEFueVByb3RvUAFaLGdvb2dsZS5nb2xhbmcub3JnL3Byb3RvYnVmL3R5cGVzL2tub3duL2FueXBiogIDR1BCqgIeR29vZ2xlLlByb3RvYnVmLldlbGxLbm93blR5cGVzYgZwcm90bzM"), 0);
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/to-binary.js
const IMPLICIT$2 = 2;
const LEGACY_REQUIRED$1 = 3;
const writeDefaults = { writeUnknownFields: true };
function makeWriteOptions$1(options) {
	return options ? Object.assign(Object.assign({}, writeDefaults), options) : writeDefaults;
}
function toBinary(schema, message, options) {
	const writer = new BinaryWriter();
	compiledWriter$1(schema)(writer, makeWriteOptions$1(options), message);
	return writer.finish();
}
const compiledWriters$1 = /* @__PURE__ */ new WeakMap();
/**
* Return the compiled encoder for a message, compiling it on first use.
*/
function compiledWriter$1(desc) {
	let compiled = compiledWriters$1.get(desc);
	if (compiled === void 0) compiled = compileMessage$2(desc);
	return compiled;
}
function compileMessage$2(desc) {
	const typeName = desc.typeName;
	const sortedFields = desc.fields.concat().sort((a, b) => a.number - b.number);
	const foreignField = sortedFields[0];
	const fieldWriters = [];
	const compiled = (writer, opts, message) => {
		if (message.$typeName !== typeName && foreignField !== void 0) throw new FieldError(foreignField, `cannot use ${foreignField} with message ${message.$typeName}`, "ForeignFieldError");
		for (let i = 0; i < fieldWriters.length; i++) fieldWriters[i](writer, opts, message);
		const unknown = message.$unknown;
		if (unknown !== void 0 && opts.writeUnknownFields) for (let i = 0; i < unknown.length; i++) {
			const { no, wireType, data } = unknown[i];
			writer.tag(no, wireType).raw(data);
		}
	};
	compiledWriters$1.set(desc, compiled);
	for (const field of sortedFields) fieldWriters.push(compileField$1(field));
	return compiled;
}
function compileField$1(field) {
	switch (field.fieldKind) {
		case "message":
		case "scalar":
		case "enum": return compileSingularField$1(field);
		case "list": return compileListField(field);
		case "map": return compileMapField(field);
	}
}
/**
* Compile an encoder for a singular field: the presence check, and the
* value encoder.
*/
function compileSingularField$1(field) {
	const writeValue = compileSingularValue$1(field);
	const localName = field.localName;
	if (field.oneof) {
		const oneofLocalName = field.oneof.localName;
		return (writer, opts, message) => {
			const oneof = message[oneofLocalName];
			if (oneof.case === localName) writeValue(writer, opts, oneof.value);
		};
	}
	if (field.presence != IMPLICIT$2) {
		const requiredError = field.presence == LEGACY_REQUIRED$1 ? `cannot encode ${field} to binary: required field not set` : void 0;
		return (writer, opts, message) => {
			const value = message[localName];
			if (value !== void 0 && Object.prototype.hasOwnProperty.call(message, localName)) writeValue(writer, opts, value);
			else if (requiredError !== void 0) throw new Error(requiredError);
		};
	}
	if (field.fieldKind == "enum") {
		const zero = field.enum.values[0].number;
		return (writer, opts, message) => {
			const value = message[localName];
			if (value !== zero) writeValue(writer, opts, value);
		};
	}
	switch (field.scalar) {
		case ScalarType.BOOL: return (writer, opts, message) => {
			const value = message[localName];
			if (value !== false) writeValue(writer, opts, value);
		};
		case ScalarType.STRING: return (writer, opts, message) => {
			const value = message[localName];
			if (value !== "") writeValue(writer, opts, value);
		};
		case ScalarType.BYTES: return (writer, opts, message) => {
			const value = message[localName];
			if (!(value instanceof Uint8Array) || value.byteLength > 0) writeValue(writer, opts, value);
		};
		case ScalarType.DOUBLE:
		case ScalarType.FLOAT: return (writer, opts, message) => {
			const value = message[localName];
			if (!Object.is(value, 0)) writeValue(writer, opts, value);
		};
		default: return (writer, opts, message) => {
			const value = message[localName];
			if (value != 0) writeValue(writer, opts, value);
		};
	}
}
/**
* Compile an encoder for the value of a singular field, including the tag.
*/
function compileSingularValue$1(field) {
	switch (field.fieldKind) {
		case "message": {
			const { toMessage } = localMessageMapper(field);
			const writeChild = compileChildWriter(field);
			return (writer, opts, value) => {
				writeChild(writer, opts, toMessage(value));
			};
		}
		case "scalar":
		case "enum": {
			const scalarType = field.fieldKind == "enum" ? ScalarType.INT32 : field.scalar;
			const fieldNo = field.number;
			const wireType = writeTypeOfScalar(scalarType);
			const writeScalar = compileScalarValue$1(scalarType, field.parent.typeName, field.name);
			return (writer, opts, value) => {
				writer.tag(fieldNo, wireType);
				writeScalar(writer, value);
			};
		}
	}
}
function compileListField(field) {
	const localName = field.localName;
	const fieldNo = field.number;
	switch (field.listKind) {
		case "message": {
			const { toMessage } = localMessageMapper(field);
			const writeChild = compileChildWriter(field);
			return (writer, opts, message) => {
				const items = message[localName];
				for (let i = 0; i < items.length; i++) writeChild(writer, opts, toMessage(items[i]));
			};
		}
		case "scalar":
		case "enum": {
			const scalarType = field.listKind == "enum" ? ScalarType.INT32 : field.scalar;
			const writeScalar = compileScalarValue$1(scalarType, field.parent.typeName, field.name);
			if (field.packed) return (writer, opts, message) => {
				const items = message[localName];
				if (items.length == 0) return;
				writer.tag(fieldNo, WireType.LengthDelimited).fork();
				for (let i = 0; i < items.length; i++) writeScalar(writer, items[i]);
				writer.join();
			};
			const wireType = writeTypeOfScalar(scalarType);
			return (writer, opts, message) => {
				const items = message[localName];
				for (let i = 0; i < items.length; i++) {
					writer.tag(fieldNo, wireType);
					writeScalar(writer, items[i]);
				}
			};
		}
	}
}
function compileMapField(field) {
	const localName = field.localName;
	const fieldNo = field.number;
	const writeKey = compileMapKey(field);
	if (field.mapKind == "message") {
		const { toMessage } = localMessageMapper(field);
		const writeMessage = compiledWriter$1(field.message);
		return (writer, opts, message) => {
			const record = message[localName];
			const keys = Object.keys(record);
			for (let i = 0; i < keys.length; i++) {
				const key = keys[i];
				writer.tag(fieldNo, WireType.LengthDelimited).fork();
				writeKey(writer, key);
				writer.tag(2, WireType.LengthDelimited).fork();
				writeMessage(writer, opts, toMessage(record[key]));
				writer.join();
				writer.join();
			}
		};
	}
	const scalarType = field.mapKind == "enum" ? ScalarType.INT32 : field.scalar;
	const valueWireType = writeTypeOfScalar(scalarType);
	const writeScalar = compileScalarValue$1(scalarType, field.parent.typeName, field.name);
	return (writer, opts, message) => {
		const record = message[localName];
		const keys = Object.keys(record);
		for (let i = 0; i < keys.length; i++) {
			const key = keys[i];
			writer.tag(fieldNo, WireType.LengthDelimited).fork();
			writeKey(writer, key);
			writer.tag(2, valueWireType);
			writeScalar(writer, record[key]);
			writer.join();
		}
	};
}
/**
* Compile an encoder for a map key. Map keys are stored as object keys and
* are always strings locally. Convert them to their scalar type before
* writing, like the reflect API does when iterating map entries.
*/
function compileMapKey(field) {
	const wireType = writeTypeOfScalar(field.mapKey);
	const writeScalar = compileScalarValue$1(field.mapKey, field.parent.typeName, field.name);
	const convertKey = compileMapKeyConverter(field.mapKey);
	return (writer, key) => {
		writer.tag(1, wireType);
		writeScalar(writer, convertKey(key));
	};
}
/**
* Returns a converter from an object key (always a string) to the closest
* possible type for the map key type. Invalid keys are passed through to
* the scalar writer, which raises an error for them.
*/
function compileMapKeyConverter(type) {
	switch (type) {
		case ScalarType.STRING: return (key) => key;
		case ScalarType.BOOL: return (key) => key === "true" ? true : key === "false" ? false : key;
		case ScalarType.UINT64:
		case ScalarType.FIXED64: return (key) => {
			try {
				return protoInt64.uParse(key);
			} catch (_a) {
				return key;
			}
		};
		case ScalarType.INT64:
		case ScalarType.SFIXED64:
		case ScalarType.SINT64: return (key) => {
			try {
				return protoInt64.parse(key);
			} catch (_a) {
				return key;
			}
		};
		default: return (key) => {
			const n = Number.parseInt(key);
			return Number.isFinite(n) ? n : key;
		};
	}
}
/**
* Compile an encoder for a bare scalar value (no tag), wrapping errors from
* the writer with the message and field name.
*/
function compileScalarValue$1(type, messageName, fieldName) {
	const writeScalar = compileScalarWrite(type);
	return (writer, value) => {
		try {
			writeScalar(writer, value);
		} catch (e) {
			if (e instanceof Error) throw new Error(`cannot encode field ${messageName}.${fieldName} to binary: ${e.message}`);
			throw e;
		}
	};
}
function compileScalarWrite(type) {
	switch (type) {
		case ScalarType.STRING: return (writer, value) => writer.string(value);
		case ScalarType.BOOL: return (writer, value) => writer.bool(value);
		case ScalarType.DOUBLE: return (writer, value) => writer.double(value);
		case ScalarType.FLOAT: return (writer, value) => writer.float(value);
		case ScalarType.INT32: return (writer, value) => writer.int32(value);
		case ScalarType.INT64: return (writer, value) => writer.int64(value);
		case ScalarType.UINT64: return (writer, value) => writer.uint64(value);
		case ScalarType.FIXED64: return (writer, value) => writer.fixed64(value);
		case ScalarType.BYTES: return (writer, value) => writer.bytes(value);
		case ScalarType.FIXED32: return (writer, value) => writer.fixed32(value);
		case ScalarType.SFIXED32: return (writer, value) => writer.sfixed32(value);
		case ScalarType.SFIXED64: return (writer, value) => writer.sfixed64(value);
		case ScalarType.SINT64: return (writer, value) => writer.sint64(value);
		case ScalarType.UINT32: return (writer, value) => writer.uint32(value);
		case ScalarType.SINT32: return (writer, value) => writer.sint32(value);
	}
}
/**
* Write a single field to binary format, if it is set. Used to serialize
* extensions: extensions always have explicit presence, so an extension
* value that was just set on the container is always written.
*
* @private
*/
function writeField(writer, opts, msg, field) {
	compileField$1(field)(writer, opts, msg[unsafeLocal]);
}
/**
* Compile an encoder for the wire format of a message field, honoring the
* delimited encoding of the field. The tag is written by the encoder.
*/
function compileChildWriter(field) {
	const fieldNo = field.number;
	const writeMessage = compiledWriter$1(field.message);
	if (field.delimitedEncoding) return (writer, opts, child) => {
		writer.tag(fieldNo, WireType.StartGroup);
		writeMessage(writer, opts, child);
		writer.tag(fieldNo, WireType.EndGroup);
	};
	return (writer, opts, child) => {
		writer.tag(fieldNo, WireType.LengthDelimited).fork();
		writeMessage(writer, opts, child);
		writer.join();
	};
}
function writeTypeOfScalar(type) {
	switch (type) {
		case ScalarType.BYTES:
		case ScalarType.STRING: return WireType.LengthDelimited;
		case ScalarType.DOUBLE:
		case ScalarType.FIXED64:
		case ScalarType.SFIXED64: return WireType.Bit64;
		case ScalarType.FIXED32:
		case ScalarType.SFIXED32:
		case ScalarType.FLOAT: return WireType.Bit32;
		default: return WireType.Varint;
	}
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/any.js
function anyPack(schema, message, into) {
	let ret = false;
	if (!into) {
		into = create$1(AnySchema);
		ret = true;
	}
	into.value = toBinary(schema, message);
	into.typeUrl = typeNameToUrl(message.$typeName);
	return ret ? into : void 0;
}
function anyIs(any, descOrTypeName) {
	if (any.typeUrl === "") return false;
	return (typeof descOrTypeName == "string" ? descOrTypeName : descOrTypeName.typeName) === typeUrlToName(any.typeUrl);
}
function anyUnpack(any, registryOrMessageDesc) {
	if (any.typeUrl === "") return;
	const desc = registryOrMessageDesc.kind == "message" ? registryOrMessageDesc : registryOrMessageDesc.getMessage(typeUrlToName(any.typeUrl));
	if (!desc || !anyIs(any, desc)) return;
	return fromBinary(desc, any.value);
}
function typeNameToUrl(name) {
	return `type.googleapis.com/${name}`;
}
function typeUrlToName(url) {
	const slash = url.lastIndexOf("/");
	const name = slash >= 0 ? url.substring(slash + 1) : url;
	if (!name.length) throw new Error(`invalid type url: ${url}`);
	return name;
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/google/protobuf/struct_pb.js
/**
* Describes the file google/protobuf/struct.proto.
*/
const file_google_protobuf_struct = /*@__PURE__*/ fileDesc("Chxnb29nbGUvcHJvdG9idWYvc3RydWN0LnByb3RvEg9nb29nbGUucHJvdG9idWYihAEKBlN0cnVjdBIzCgZmaWVsZHMYASADKAsyIy5nb29nbGUucHJvdG9idWYuU3RydWN0LkZpZWxkc0VudHJ5GkUKC0ZpZWxkc0VudHJ5EgsKA2tleRgBIAEoCRIlCgV2YWx1ZRgCIAEoCzIWLmdvb2dsZS5wcm90b2J1Zi5WYWx1ZToCOAEi6gEKBVZhbHVlEjAKCm51bGxfdmFsdWUYASABKA4yGi5nb29nbGUucHJvdG9idWYuTnVsbFZhbHVlSAASFgoMbnVtYmVyX3ZhbHVlGAIgASgBSAASFgoMc3RyaW5nX3ZhbHVlGAMgASgJSAASFAoKYm9vbF92YWx1ZRgEIAEoCEgAEi8KDHN0cnVjdF92YWx1ZRgFIAEoCzIXLmdvb2dsZS5wcm90b2J1Zi5TdHJ1Y3RIABIwCgpsaXN0X3ZhbHVlGAYgASgLMhouZ29vZ2xlLnByb3RvYnVmLkxpc3RWYWx1ZUgAQgYKBGtpbmQiMwoJTGlzdFZhbHVlEiYKBnZhbHVlcxgBIAMoCzIWLmdvb2dsZS5wcm90b2J1Zi5WYWx1ZSobCglOdWxsVmFsdWUSDgoKTlVMTF9WQUxVRRAAQn8KE2NvbS5nb29nbGUucHJvdG9idWZCC1N0cnVjdFByb3RvUAFaL2dvb2dsZS5nb2xhbmcub3JnL3Byb3RvYnVmL3R5cGVzL2tub3duL3N0cnVjdHBi+AEBogIDR1BCqgIeR29vZ2xlLlByb3RvYnVmLldlbGxLbm93blR5cGVzYgZwcm90bzM");
/**
* Describes the message google.protobuf.Struct.
* Use `create(StructSchema)` to create a new message.
*/
const StructSchema = /*@__PURE__*/ messageDesc(file_google_protobuf_struct, 0);
/**
* Describes the message google.protobuf.Value.
* Use `create(ValueSchema)` to create a new message.
*/
const ValueSchema = /*@__PURE__*/ messageDesc(file_google_protobuf_struct, 1);
/**
* Describes the message google.protobuf.ListValue.
* Use `create(ListValueSchema)` to create a new message.
*/
const ListValueSchema = /*@__PURE__*/ messageDesc(file_google_protobuf_struct, 2);
/**
* Represents a JSON `null`.
*
* `NullValue` is a sentinel, using an enum with only one value to represent
* the null value for the `Value` type union.
*
* A field of type `NullValue` with any value other than `0` is considered
* invalid. Most ProtoJSON serializers will emit a `Value` with a `null_value`
* set as a JSON `null` regardless of the integer value, and so will round trip
* to a `0` value.
*
* @generated from enum google.protobuf.NullValue
*/
var NullValue;
(function(NullValue) {
	/**
	* Null value.
	*
	* @generated from enum value: NULL_VALUE = 0;
	*/
	NullValue[NullValue["NULL_VALUE"] = 0] = "NULL_VALUE";
})(NullValue || (NullValue = {}));
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/extensions.js
/**
* Retrieve an extension value from a message.
*
* The function never returns undefined. Use hasExtension() to check whether an
* extension is set. If the extension is not set, this function returns the
* default value (if one was specified in the protobuf source), or the zero value
* (for example `0` for numeric types, `[]` for repeated extension fields, and
* an empty message instance for message fields).
*
* Extensions are stored as unknown fields on a message. To mutate an extension
* value, make sure to store the new value with setExtension() after mutating.
*
* If the extension does not extend the given message, an error is raised.
*/
function getExtension(message, extension, options) {
	assertExtendee(extension, message);
	const ufs = filterUnknownFields(message.$unknown, extension);
	const [container, field, get] = createExtensionContainer(extension);
	const ctx = makeReadContext$1(options);
	for (const uf of ufs) readField(container, new BinaryReader(uf.data), field, uf.wireType, ctx);
	return get();
}
/**
* Set an extension value on a message. If the message already has a value for
* this extension, the value is replaced.
*
* If the extension does not extend the given message, an error is raised.
*/
function setExtension(message, extension, value) {
	var _a;
	assertExtendee(extension, message);
	const ufs = ((_a = message.$unknown) !== null && _a !== void 0 ? _a : []).filter((uf) => uf.no !== extension.number);
	const [container, field] = createExtensionContainer(extension, value);
	const writer = new BinaryWriter();
	writeField(writer, { writeUnknownFields: true }, container, field);
	const reader = new BinaryReader(writer.finish());
	while (reader.pos < reader.len) {
		const [no, wireType] = reader.tag();
		const data = reader.skip(wireType, no);
		ufs.push({
			no,
			wireType,
			data
		});
	}
	message.$unknown = ufs;
}
function filterUnknownFields(unknownFields, extension) {
	if (unknownFields === void 0) return [];
	if (extension.fieldKind === "enum" || extension.fieldKind === "scalar") {
		for (let i = unknownFields.length - 1; i >= 0; --i) if (unknownFields[i].no == extension.number) return [unknownFields[i]];
		return [];
	}
	return unknownFields.filter((uf) => uf.no === extension.number);
}
/**
* @private
*/
function createExtensionContainer(extension, value) {
	const localName = extension.typeName;
	const field = Object.assign(Object.assign({}, extension), {
		kind: "field",
		parent: extension.extendee,
		localName
	});
	const desc = Object.assign(Object.assign({}, extension.extendee), {
		fields: [field],
		members: [field],
		oneofs: []
	});
	const container = create$1(desc, value !== void 0 ? { [localName]: value } : void 0);
	return [
		reflect(desc, container),
		field,
		() => {
			const value = container[localName];
			if (value === void 0) {
				const desc = extension.message;
				if (isWrapperDesc(desc)) return scalarZeroValue(desc.fields[0].scalar, desc.fields[0].longAsString);
				return create$1(desc);
			}
			return value;
		}
	];
}
function assertExtendee(extension, message) {
	if (extension.extendee.typeName != message.$typeName) throw new Error(`extension ${extension.typeName} can only be applied to message ${extension.extendee.typeName}`);
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/wkt/json.js
/**
* Minimum google.protobuf.Timestamp in milliseconds (inclusive).
* Only enforced in ProtoJSON.
*
* @private
*/
const timestampMsMin = /*@__PURE__*/ Date.parse("0001-01-01T00:00:00Z");
/**
* Maximum google.protobuf.Timestamp in milliseconds (inclusive).
* Only enforced in ProtoJSON.
*
* @private
*/
const timestampMsMax = /*@__PURE__*/ Date.parse("9999-12-31T23:59:59Z");
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/to-json.js
const LEGACY_REQUIRED = 3;
const IMPLICIT$1 = 2;
const jsonWriteDefaults = {
	alwaysEmitImplicit: false,
	enumAsInteger: false,
	useProtoFieldName: false
};
function makeWriteOptions(options) {
	return options ? Object.assign(Object.assign({}, jsonWriteDefaults), options) : jsonWriteDefaults;
}
/**
* Serialize the message to a JSON value, a JavaScript value that can be
* passed to JSON.stringify().
*/
function toJson(schema, message, options) {
	return compiledWriter(schema)(makeWriteOptions(options), message);
}
/**
* Serialize the message to a JSON string.
*/
function toJsonString(schema, message, options) {
	var _a;
	const jsonValue = toJson(schema, message, options);
	return JSON.stringify(jsonValue, null, (_a = options === null || options === void 0 ? void 0 : options.prettySpaces) !== null && _a !== void 0 ? _a : 0);
}
const compiledWriters = /* @__PURE__ */ new WeakMap();
/**
* Return the compiled encoder for a message, compiling it on first use.
*/
function compiledWriter(desc) {
	let compiled = compiledWriters.get(desc);
	if (compiled === void 0) compiled = compileMessage$1(desc);
	return compiled;
}
function compileMessage$1(desc) {
	const typeName = desc.typeName;
	const writeWkt = compileWkt$1(desc);
	if (writeWkt !== void 0) {
		const foreignField = desc.fields[0];
		const compiledWriter = (opts, message) => {
			if (message.$typeName !== typeName && foreignField !== void 0) throw new FieldError(foreignField, `cannot use ${foreignField} with message ${message.$typeName}`, "ForeignFieldError");
			return writeWkt(opts, message);
		};
		compiledWriters.set(desc, compiledWriter);
		return compiledWriter;
	}
	const sortedFields = desc.fields.concat().sort((a, b) => a.number - b.number);
	const foreignField = sortedFields[0];
	const fieldWriters = [];
	const compiledWriter = (opts, message) => {
		if (message.$typeName !== typeName && foreignField !== void 0) throw new FieldError(foreignField, `cannot use ${foreignField} with message ${message.$typeName}`, "ForeignFieldError");
		const json = {};
		for (let i = 0; i < fieldWriters.length; i++) fieldWriters[i](opts, message, json);
		if (opts.registry) writeExtensions(json, opts, opts.registry, message, desc);
		return json;
	};
	compiledWriters.set(desc, compiledWriter);
	for (const field of sortedFields) {
		const writeField = compileField(field);
		fieldWriters.push(field.jsonName === "__proto__" || field.name === "__proto__" ? writeOwnKeys(writeField) : writeField);
	}
	return compiledWriter;
}
/**
* Wrap a field encoder so that a JSON name or proto name of "__proto__" ends
* up as an own property. Assigning that key on a plain object hits the
* inherited accessor instead, and the field is silently lost. The encoder
* writes to a null-prototype object, where the key is ordinary, and the
* result is copied to the real output with setOwn().
*/
function writeOwnKeys(writeField) {
	return (opts, message, json) => {
		const tmp = Object.create(null);
		writeField(opts, message, tmp);
		for (const key of Object.keys(tmp)) setOwn(json, key, tmp[key]);
	};
}
/**
* Compile an encoder for a well-known type with a custom JSON representation,
* or return undefined for other messages.
*/
function compileWkt$1(desc) {
	if (!desc.typeName.startsWith("google.protobuf.")) return;
	switch (desc.typeName) {
		case "google.protobuf.Any": return (opts, message) => anyToJson(message, opts);
		case "google.protobuf.Timestamp": return (opts, message) => timestampToJson(message);
		case "google.protobuf.Duration": return (opts, message) => durationToJson(message);
		case "google.protobuf.FieldMask": return (opts, message) => fieldMaskToJson(message);
		case "google.protobuf.Struct": return (opts, message) => structToJson(message);
		case "google.protobuf.Value": return (opts, message) => valueToJson(message);
		case "google.protobuf.ListValue": return (opts, message) => listValueToJson(message);
		default:
			if (isWrapperDesc(desc)) {
				const valueField = desc.fields[0];
				const localName = valueField.localName;
				const zero = scalarZeroValue(valueField.scalar, false);
				const writeScalar = compileScalarValue(valueField);
				return (opts, message) => {
					const value = message[localName];
					return writeScalar(opts, value === void 0 ? zero : value);
				};
			}
			return;
	}
}
function compileField(field) {
	switch (field.fieldKind) {
		case "scalar":
		case "enum":
		case "message": return compileSingularField(field);
		case "list":
		case "map": {
			const writeValue = field.fieldKind == "list" ? compileListValue(field) : compileMapValue(field);
			const protoName = field.name;
			const jsonKey = field.jsonName;
			const localName = field.localName;
			return (opts, message, json) => {
				const value = writeValue(opts, message[localName]);
				if (value !== void 0) json[opts.useProtoFieldName ? protoName : jsonKey] = value;
			};
		}
	}
}
/**
* Compile an encoder for a singular field: the presence check, and the
* value encoder.
*/
function compileSingularField(field) {
	const writeValue = compileSingularValue(field);
	const protoName = field.name;
	const jsonKey = field.jsonName;
	const localName = field.localName;
	if (field.oneof) {
		const oneofLocalName = field.oneof.localName;
		return (opts, message, json) => {
			const oneof = message[oneofLocalName];
			if (oneof.case === localName) json[opts.useProtoFieldName ? protoName : jsonKey] = writeValue(opts, oneof.value);
		};
	}
	if (field.presence != IMPLICIT$1) {
		const requiredError = field.presence == LEGACY_REQUIRED ? `cannot encode ${field} to JSON: required field not set` : void 0;
		return (opts, message, json) => {
			const value = message[localName];
			if (value !== void 0 && Object.prototype.hasOwnProperty.call(message, localName)) json[opts.useProtoFieldName ? protoName : jsonKey] = writeValue(opts, value);
			else if (requiredError !== void 0) throw new Error(requiredError);
		};
	}
	if (field.fieldKind == "enum") {
		const zero = field.enum.values[0].number;
		return (opts, message, json) => {
			const value = message[localName];
			if (value !== zero || opts.alwaysEmitImplicit) json[opts.useProtoFieldName ? protoName : jsonKey] = writeValue(opts, value);
		};
	}
	switch (field.scalar) {
		case ScalarType.BOOL: return (opts, message, json) => {
			const value = message[localName];
			if (value !== false || opts.alwaysEmitImplicit) json[opts.useProtoFieldName ? protoName : jsonKey] = writeValue(opts, value);
		};
		case ScalarType.STRING: return (opts, message, json) => {
			const value = message[localName];
			if (value !== "" || opts.alwaysEmitImplicit) json[opts.useProtoFieldName ? protoName : jsonKey] = writeValue(opts, value);
		};
		case ScalarType.BYTES: return (opts, message, json) => {
			const value = message[localName];
			if (!(value instanceof Uint8Array) || value.byteLength > 0 || opts.alwaysEmitImplicit) json[opts.useProtoFieldName ? protoName : jsonKey] = writeValue(opts, value);
		};
		case ScalarType.DOUBLE:
		case ScalarType.FLOAT: return (opts, message, json) => {
			const value = message[localName];
			if (!Object.is(value, 0) || opts.alwaysEmitImplicit) json[opts.useProtoFieldName ? protoName : jsonKey] = writeValue(opts, value);
		};
		default: return (opts, message, json) => {
			const value = message[localName];
			if (value != 0 || opts.alwaysEmitImplicit) json[opts.useProtoFieldName ? protoName : jsonKey] = writeValue(opts, value);
		};
	}
}
/**
* Compile an encoder for the value of a field of any kind. Used for
* extension values.
*/
function compileFieldValue(field) {
	switch (field.fieldKind) {
		case "scalar":
		case "enum":
		case "message": return compileSingularValue(field);
		case "list": return compileListValue(field);
		case "map": return compileMapValue(field);
	}
}
/**
* Compile an encoder for the value of a singular field.
*/
function compileSingularValue(field) {
	switch (field.fieldKind) {
		case "scalar": return compileScalarValue(field);
		case "enum": return compileEnumValue(field);
		case "message": return compileMessageValue(field);
	}
}
/**
* Compile an encoder for the value of a message field.
*/
function compileMessageValue(field) {
	const { toMessage } = localMessageMapper(field);
	const writeMessage = compiledWriter(field.message);
	return (opts, value) => writeMessage(opts, toMessage(value));
}
/**
* Compile an encoder for a list field value. Returns undefined for an empty
* list, unless alwaysEmitImplicit is enabled.
*/
function compileListValue(field) {
	const writeItem = compileListItemValue(field);
	return (opts, value) => {
		const items = value;
		if (items.length == 0 && !opts.alwaysEmitImplicit) return;
		const jsonArray = [];
		for (let i = 0; i < items.length; i++) jsonArray.push(writeItem(opts, items[i]));
		return jsonArray;
	};
}
function compileListItemValue(field) {
	switch (field.listKind) {
		case "scalar": return compileScalarValue(field);
		case "enum": return compileEnumValue(field);
		case "message": return compileMessageValue(field);
	}
}
/**
* Compile an encoder for a map field value. Returns undefined for an empty
* map, unless alwaysEmitImplicit is enabled. Map keys are stored as object
* keys and are used as JSON keys as-is.
*/
function compileMapValue(field) {
	const writeMapValue = compileMapEntryValue(field);
	return (opts, value) => {
		const record = value;
		const keys = Object.keys(record);
		if (keys.length == 0 && !opts.alwaysEmitImplicit) return;
		const jsonObject = {};
		for (let i = 0; i < keys.length; i++) {
			const key = keys[i];
			setOwn(jsonObject, key, writeMapValue(opts, record[key]));
		}
		return jsonObject;
	};
}
function compileMapEntryValue(field) {
	switch (field.mapKind) {
		case "scalar": return compileScalarValue(field);
		case "enum": return compileEnumValue(field);
		case "message": return compileMessageValue(field);
	}
}
/**
* Compile an encoder for an enum value.
*/
function compileEnumValue(field) {
	const desc = field.enum;
	if (desc.typeName == "google.protobuf.NullValue") return (opts, value) => {
		if (typeof value != "number") throw errorEnumValue(desc, value);
		return null;
	};
	return (opts, value) => {
		var _a, _b;
		if (typeof value != "number") throw errorEnumValue(desc, value);
		if (opts.enumAsInteger) return value;
		return (_b = (_a = desc.value[value]) === null || _a === void 0 ? void 0 : _a.jsonName) !== null && _b !== void 0 ? _b : value;
	};
}
function errorEnumValue(desc, value) {
	return /* @__PURE__ */ new Error(`cannot encode ${desc} to JSON: expected number, got ${formatVal(value)}`);
}
/**
* Compile an encoder for a scalar value. Errors report the original field
* descriptor, which may be a list or map field for items of those fields.
*/
function compileScalarValue(field) {
	switch (field.scalar) {
		case ScalarType.INT32:
		case ScalarType.SFIXED32:
		case ScalarType.SINT32:
		case ScalarType.FIXED32:
		case ScalarType.UINT32: return (opts, value) => {
			if (typeof value != "number") throw errorScalarValue(field, value);
			return value;
		};
		case ScalarType.FLOAT:
		case ScalarType.DOUBLE: return (opts, value) => {
			if (typeof value != "number") throw errorScalarValue(field, value);
			if (Number.isNaN(value)) return "NaN";
			if (value === Number.POSITIVE_INFINITY) return "Infinity";
			if (value === Number.NEGATIVE_INFINITY) return "-Infinity";
			return value;
		};
		case ScalarType.STRING: return (opts, value) => {
			if (typeof value != "string") throw errorScalarValue(field, value);
			return value;
		};
		case ScalarType.BOOL: return (opts, value) => {
			if (typeof value != "boolean") throw errorScalarValue(field, value);
			return value;
		};
		case ScalarType.UINT64:
		case ScalarType.FIXED64:
		case ScalarType.INT64:
		case ScalarType.SFIXED64:
		case ScalarType.SINT64: return (opts, value) => {
			if (typeof value == "bigint" || typeof value == "string" || typeof value == "number" && Number.isInteger(value)) return value.toString();
			throw errorScalarValue(field, value);
		};
		case ScalarType.BYTES: return (opts, value) => {
			if (value instanceof Uint8Array) return base64Encode(value);
			throw errorScalarValue(field, value);
		};
	}
}
function errorScalarValue(field, value) {
	var _a;
	return /* @__PURE__ */ new Error(`cannot encode ${field} to JSON: ${(_a = checkField(field, value)) === null || _a === void 0 ? void 0 : _a.message}`);
}
/**
* Write extensions for unknown fields that are found in the registry.
*/
function writeExtensions(json, opts, registry, message, desc) {
	const unknown = message.$unknown;
	if (unknown === void 0) return;
	const tagSeen = /* @__PURE__ */ new Set();
	for (let i = 0; i < unknown.length; i++) {
		const { no } = unknown[i];
		if (!tagSeen.has(no)) {
			tagSeen.add(no);
			const extension = registry.getExtensionFor(desc, no);
			if (!extension) continue;
			const [container, field] = createExtensionContainer(extension, getExtension(message, extension));
			const local = container[unsafeLocal];
			const jsonValue = compileFieldValue(field)(opts, local[field.localName]);
			if (jsonValue !== void 0) json[extension.jsonName] = jsonValue;
		}
	}
}
function anyToJson(val, opts) {
	if (val.typeUrl === "") return {};
	const { registry } = opts;
	let message;
	let desc;
	if (registry) {
		message = anyUnpack(val, registry);
		if (message) desc = registry.getMessage(message.$typeName);
	}
	if (!desc || !message) throw new Error(`cannot encode message ${val.$typeName} to JSON: "${val.typeUrl}" is not in the type registry`);
	const json = hasCustomJsonRepresentation(desc) ? { value: compiledWriter(desc)(opts, message) } : compiledWriter(desc)(opts, message);
	json["@type"] = val.typeUrl;
	return json;
}
function durationToJson(val) {
	const seconds = Number(val.seconds);
	const nanos = val.nanos;
	if (seconds > 315576e6 || seconds < -315576e6) throw new Error(`cannot encode message ${val.$typeName} to JSON: value out of range`);
	if (seconds > 0 && nanos < 0 || seconds < 0 && nanos > 0) throw new Error(`cannot encode message ${val.$typeName} to JSON: nanos sign must match seconds sign`);
	let text = val.seconds.toString();
	if (nanos !== 0) {
		let nanosStr = Math.abs(nanos).toString();
		nanosStr = "0".repeat(9 - nanosStr.length) + nanosStr;
		if (nanosStr.substring(3) === "000000") nanosStr = nanosStr.substring(0, 3);
		else if (nanosStr.substring(6) === "000") nanosStr = nanosStr.substring(0, 6);
		text += "." + nanosStr;
		if (nanos < 0 && seconds == 0) text = "-" + text;
	}
	return text + "s";
}
function fieldMaskToJson(val) {
	return val.paths.map((p) => {
		if (protoSnakeCase(protoCamelCase(p)) !== p) throw new Error(`cannot encode message ${val.$typeName} to JSON: lowerCamelCase of path name "${p}" is irreversible`);
		return protoCamelCase(p);
	}).join(",");
}
function structToJson(val) {
	const json = {};
	const keys = Object.keys(val.fields);
	for (let i = 0; i < keys.length; i++) {
		const key = keys[i];
		setOwn(json, key, valueToJson(val.fields[key]));
	}
	return json;
}
function valueToJson(val) {
	switch (val.kind.case) {
		case "nullValue": return null;
		case "numberValue":
			if (!Number.isFinite(val.kind.value)) throw new Error(`${val.$typeName} cannot be NaN or Infinity`);
			return val.kind.value;
		case "boolValue": return val.kind.value;
		case "stringValue": return val.kind.value;
		case "structValue": return structToJson(val.kind.value);
		case "listValue": return listValueToJson(val.kind.value);
		default: throw new Error(`${val.$typeName} must have a value`);
	}
}
function listValueToJson(val) {
	return val.values.map(valueToJson);
}
function timestampToJson(val) {
	const ms = Number(val.seconds) * 1e3;
	if (ms < timestampMsMin || ms > timestampMsMax) throw new Error(`cannot encode message ${val.$typeName} to JSON: must be from 0001-01-01T00:00:00Z to 9999-12-31T23:59:59Z inclusive`);
	if (val.nanos < 0) throw new Error(`cannot encode message ${val.$typeName} to JSON: nanos must not be negative`);
	if (val.nanos > 999999999) throw new Error(`cannot encode message ${val.$typeName} to JSON: nanos must not be greater than 99999999`);
	let z = "Z";
	if (val.nanos > 0) {
		const nanosStr = (val.nanos + 1e9).toString().substring(1);
		if (nanosStr.substring(3) === "000000") z = "." + nanosStr.substring(0, 3) + "Z";
		else if (nanosStr.substring(6) === "000") z = "." + nanosStr.substring(0, 6) + "Z";
		else z = "." + nanosStr + "Z";
	}
	return new Date(ms).toISOString().replace(".000Z", z);
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/from-json.js
const IMPLICIT = 2;
function makeReadContext(options) {
	return Object.assign(Object.assign({
		ignoreUnknownFields: false,
		recursionLimit: 100
	}, options), { depth: 0 });
}
/**
* Parse a message from a JSON string.
*
* Duplicate keys are rejected.
*/
function fromJsonString(schema, json, options) {
	return fromJson(schema, parseJsonString(json, schema.typeName), options);
}
/**
* Parse a message from a JSON value.
*
* Duplicate keys are rejected, but a value parsed by JSON.parse has already
* dropped duplicates (the last one wins). Use `fromJsonString` for strict
* duplicate-key checking.
*/
function fromJson(schema, json, options) {
	const message = create$1(schema);
	readMessage(schema, message, json, options);
	return message;
}
/**
* Run the compiled decoder for the message, wrapping FieldErrors with the
* standard error message.
*/
function readMessage(schema, message, json, options) {
	try {
		compiledReader(schema)(message, json, makeReadContext(options));
	} catch (e) {
		if (isFieldError(e)) throw new Error(`cannot decode ${e.field()} from JSON: ${e.message}`, { cause: e });
		throw e;
	}
}
const compiledReaders = /* @__PURE__ */ new WeakMap();
/**
* Return the compiled decoder for a message, compiling it on first use.
*/
function compiledReader(desc) {
	let compiled = compiledReaders.get(desc);
	if (compiled === void 0) compiled = compileMessage(desc);
	return compiled;
}
function compileMessage(desc) {
	const descString = String(desc);
	const readWkt = compileWkt(desc);
	if (readWkt !== void 0) {
		const compiled = (message, json, ctx) => {
			if (++ctx.depth > ctx.recursionLimit) throw new Error(`cannot decode ${descString} from JSON: maximum recursion depth of ${ctx.recursionLimit} reached`);
			readWkt(message, json, ctx);
			ctx.depth--;
		};
		compiledReaders.set(desc, compiled);
		return compiled;
	}
	const typeName = desc.typeName;
	const fieldsByJsonKey = /* @__PURE__ */ new Map();
	const compiled = (message, json, ctx) => {
		var _a;
		if (++ctx.depth > ctx.recursionLimit) throw new Error(`cannot decode ${descString} from JSON: maximum recursion depth of ${ctx.recursionLimit} reached`);
		if (json == null || Array.isArray(json) || typeof json != "object") throw new Error(`cannot decode ${descString} from JSON: ${formatVal(json)}`);
		const oneofSeen = /* @__PURE__ */ new Map();
		const fieldSeen = /* @__PURE__ */ new Set();
		const jsonKeys = Object.keys(json);
		for (let i = 0; i < jsonKeys.length; i++) {
			const jsonKey = jsonKeys[i];
			const jsonValue = json[jsonKey];
			const entry = fieldsByJsonKey.get(jsonKey);
			if (entry !== void 0) {
				const field = entry.field;
				if (fieldSeen.has(field)) throw new FieldError(field, "set multiple times");
				fieldSeen.add(field);
				if (entry.oneofScalarNullSkip && jsonValue === null) continue;
				if (entry.oneof) {
					const seen = oneofSeen.get(entry.oneof);
					if (seen !== void 0) throw new FieldError(entry.oneof, `oneof set multiple times by ${seen.name} and ${field.name}`);
					oneofSeen.set(entry.oneof, field);
				}
				entry.read(message, jsonValue, ctx);
			} else {
				const extension = jsonKey.startsWith("[") && jsonKey.endsWith("]") ? (_a = ctx.registry) === null || _a === void 0 ? void 0 : _a.getExtension(jsonKey.substring(1, jsonKey.length - 1)) : void 0;
				if ((extension === null || extension === void 0 ? void 0 : extension.extendee.typeName) == typeName) {
					const [container, field, get] = createExtensionContainer(extension);
					compileFieldReader(field)(container[unsafeLocal], jsonValue, ctx);
					setExtension(message, extension, get());
				}
				if (extension === void 0 && !ctx.ignoreUnknownFields) throw new Error(`cannot decode ${descString} from JSON: key "${jsonKey}" is unknown`);
			}
		}
		ctx.depth--;
	};
	compiledReaders.set(desc, compiled);
	for (const field of desc.fields) {
		const entry = {
			read: compileFieldReader(field),
			field,
			oneof: field.oneof,
			oneofScalarNullSkip: field.oneof !== void 0 && field.fieldKind == "scalar"
		};
		fieldsByJsonKey.set(field.name, entry).set(field.jsonName, entry);
	}
	return compiled;
}
/**
* Compile a decoder for a well-known type with a custom JSON representation,
* or return undefined for other messages. The recursion limit is enforced by
* the caller.
*/
function compileWkt(desc) {
	if (!desc.typeName.startsWith("google.protobuf.")) return;
	switch (desc.typeName) {
		case "google.protobuf.Any": return (message, json, ctx) => anyFromJson(message, json, ctx);
		case "google.protobuf.Timestamp": return (message, json) => timestampFromJson(message, json);
		case "google.protobuf.Duration": return (message, json) => durationFromJson(message, json);
		case "google.protobuf.FieldMask": return (message, json) => fieldMaskFromJson(message, json);
		case "google.protobuf.Struct": return (message, json, ctx) => structFromJson(message, json, ctx);
		case "google.protobuf.Value": return (message, json, ctx) => valueFromJson(message, json, ctx);
		case "google.protobuf.ListValue": return (message, json, ctx) => listValueFromJson(message, json, ctx);
		default:
			if (isWrapperDesc(desc)) {
				const valueField = desc.fields[0];
				const localName = valueField.localName;
				const scalar = valueField.scalar;
				const longAsString = valueField.longAsString;
				const readScalar = compileScalarConverter(valueField);
				return (message, json) => {
					if (json === null) message[localName] = scalarZeroValue(scalar, longAsString);
					else message[localName] = readScalar(json);
				};
			}
			return;
	}
}
function compileFieldReader(field) {
	switch (field.fieldKind) {
		case "scalar": return compileScalarFieldReader(field);
		case "enum": return compileEnumFieldReader(field);
		case "message": return compileMessageFieldReader(field);
		case "list": return compileListFieldReader(field);
		case "map": return compileMapFieldReader(field);
	}
}
function compileScalarFieldReader(field) {
	const readScalar = compileScalarConverter(field);
	const localName = field.localName;
	if (field.oneof) {
		const oneofLocalName = field.oneof.localName;
		return (message, json) => {
			message[oneofLocalName] = {
				case: localName,
				value: readScalar(json)
			};
		};
	}
	const clear = compileClear(field);
	return (message, json) => {
		if (json === null) clear(message);
		else message[localName] = readScalar(json);
	};
}
/**
* Compile a function that resets the field to unset, mirroring the clear
* operation of the reflect API for fields that are not part of a oneof.
*/
function compileClear(field) {
	const localName = field.localName;
	if (field.presence != IMPLICIT) return (message) => {
		delete message[localName];
	};
	if (field.fieldKind == "enum") {
		const zero = field.enum.values[0].number;
		return (message) => {
			message[localName] = zero;
		};
	}
	const scalar = field.scalar;
	const longAsString = field.longAsString;
	return (message) => {
		message[localName] = scalarZeroValue(scalar, longAsString);
	};
}
function compileEnumFieldReader(field) {
	const readEnumValue = compileEnumConverter(field.enum);
	const checkEnum = compileEnumCheck(field.enum);
	const localName = field.localName;
	const nullResets = field.enum.typeName != "google.protobuf.NullValue";
	if (field.oneof) {
		const oneofLocalName = field.oneof.localName;
		return (message, json, ctx) => {
			if (json === null && nullResets) {
				if (message[oneofLocalName].case === localName) message[oneofLocalName] = { case: void 0 };
				return;
			}
			const value = readEnumValue(json, ctx.ignoreUnknownFields);
			if (value === tokenIgnoredUnknownEnum) return;
			const check = checkEnum(value);
			if (check !== true) throw new FieldError(field, reasonSingular(field, value, check));
			message[oneofLocalName] = {
				case: localName,
				value
			};
		};
	}
	const clear = compileClear(field);
	return (message, json, ctx) => {
		if (json === null && nullResets) {
			clear(message);
			return;
		}
		const value = readEnumValue(json, ctx.ignoreUnknownFields);
		if (value === tokenIgnoredUnknownEnum) return;
		const check = checkEnum(value);
		if (check !== true) throw new FieldError(field, reasonSingular(field, value, check));
		message[localName] = value;
	};
}
function compileMessageFieldReader(field) {
	const localName = field.localName;
	const { toMessage, toLocal } = localMessageMapper(field);
	const readChild = compiledReader(field.message);
	const nullResets = field.message.typeName != "google.protobuf.Value";
	if (field.oneof) {
		const oneofLocalName = field.oneof.localName;
		return (message, json, ctx) => {
			const oneof = message[oneofLocalName];
			if (json === null && nullResets) {
				if (oneof.case === localName) message[oneofLocalName] = { case: void 0 };
				return;
			}
			const child = toMessage(oneof.case === localName ? oneof.value : void 0);
			readChild(child, json, ctx);
			message[oneofLocalName] = {
				case: localName,
				value: toLocal(child)
			};
		};
	}
	return (message, json, ctx) => {
		if (json === null && nullResets) {
			delete message[localName];
			return;
		}
		const child = toMessage(message[localName]);
		readChild(child, json, ctx);
		message[localName] = toLocal(child);
	};
}
function compileListFieldReader(field) {
	const localName = field.localName;
	const readItem = compileListItemReader(field);
	return (message, json, ctx) => {
		if (json === null) return;
		if (!Array.isArray(json)) throw new FieldError(field, "expected Array, got " + formatVal(json));
		const items = message[localName];
		for (let i = 0; i < json.length; i++) {
			const value = readItem(json[i], ctx, items.length);
			if (value !== tokenIgnoredUnknownEnum) items.push(value);
		}
	};
}
/**
* Compile a decoder for a list item. The index is only used in errors, and
* accounts for previously merged items.
*/
function compileListItemReader(field) {
	switch (field.listKind) {
		case "scalar": {
			const parseScalar = compileScalarParse(field);
			const checkValue = checkScalarValue(field.scalar);
			const toLocal = compileScalarToLocal(field);
			return (json, ctx, index) => {
				if (json === null) throw new FieldError(field, "list item must not be null");
				const value = parseScalar(json);
				const check = checkValue(value);
				if (check !== true) throw new FieldError(field, `list item #${index + 1}: ${reasonSingular(field, value, check)}`);
				return toLocal(value);
			};
		}
		case "enum": {
			const readEnumValue = compileEnumConverter(field.enum);
			const checkEnum = compileEnumCheck(field.enum);
			const nullResets = field.enum.typeName != "google.protobuf.NullValue";
			return (json, ctx, index) => {
				if (json === null && nullResets) throw new FieldError(field, "list item must not be null");
				const value = readEnumValue(json, ctx.ignoreUnknownFields);
				if (value === tokenIgnoredUnknownEnum) return value;
				const check = checkEnum(value);
				if (check !== true) throw new FieldError(field, `list item #${index + 1}: ${reasonSingular(field, value, check)}`);
				return value;
			};
		}
		case "message": {
			const { toMessage, toLocal } = localMessageMapper(field);
			const readChild = compiledReader(field.message);
			const nullResets = field.message.typeName != "google.protobuf.Value";
			return (json, ctx) => {
				if (json === null && nullResets) throw new FieldError(field, "list item must not be null");
				const child = toMessage(void 0);
				readChild(child, json, ctx);
				return toLocal(child);
			};
		}
	}
}
function compileMapFieldReader(field) {
	const localName = field.localName;
	const mapKey = field.mapKey;
	const parseMapKey = compileMapKeyParse(mapKey);
	const checkMapKey = checkScalarValue(mapKey);
	let parseValue;
	let checkValue;
	let toLocalValue = (value) => value;
	let nullResets = true;
	switch (field.mapKind) {
		case "scalar":
			parseValue = compileScalarParse(field);
			checkValue = checkScalarValue(field.scalar);
			toLocalValue = compileScalarToLocal(field);
			break;
		case "enum": {
			const readEnumValue = compileEnumConverter(field.enum);
			parseValue = (json, ctx) => readEnumValue(json, ctx.ignoreUnknownFields);
			checkValue = compileEnumCheck(field.enum);
			nullResets = field.enum.typeName != "google.protobuf.NullValue";
			break;
		}
		case "message": {
			const { toMessage, toLocal } = localMessageMapper(field);
			const readChild = compiledReader(field.message);
			nullResets = field.message.typeName != "google.protobuf.Value";
			parseValue = (json, ctx) => {
				const child = toMessage(void 0);
				readChild(child, json, ctx);
				return toLocal(child);
			};
			break;
		}
	}
	return (message, json, ctx) => {
		if (json === null) return;
		if (typeof json != "object" || Array.isArray(json)) throw new FieldError(field, "expected object, got " + formatVal(json));
		const record = message[localName];
		const seen = /* @__PURE__ */ new Set();
		const jsonMapKeys = Object.keys(json);
		for (let i = 0; i < jsonMapKeys.length; i++) {
			const jsonMapKey = jsonMapKeys[i];
			const jsonMapValue = json[jsonMapKey];
			const key = parseMapKey(jsonMapKey);
			if (seen.has(key)) throw new FieldError(field, `duplicate map key "${jsonMapKey}"`);
			seen.add(key);
			if (jsonMapValue === null && nullResets) throw new FieldError(field, "map value must not be null");
			const value = parseValue(jsonMapValue, ctx);
			if (value === tokenIgnoredUnknownEnum) continue;
			const checkKey = checkMapKey(key);
			if (checkKey !== true) throw new FieldError(field, `invalid map key: ${reasonSingular({ scalar: mapKey }, key, checkKey)}`);
			if (checkValue !== void 0) {
				const check = checkValue(value);
				if (check !== true) throw new FieldError(field, `map entry ${formatVal(key)}: ${reasonSingular(field, value, check)}`);
			}
			setOwn(record, key, toLocalValue(value));
		}
	};
}
const tokenIgnoredUnknownEnum = Symbol();
/**
* Compile a converter from a JSON value to an enum value. JSON null returns
* the enum's first value. With ignoreUnknownFields false, unknown string
* values raise an error; with true, they return tokenIgnoredUnknownEnum.
* The value is not checked against the enum's values, see compileEnumCheck.
*/
function compileEnumConverter(desc) {
	const zero = desc.values[0].number;
	const numbers = /* @__PURE__ */ new Map();
	for (const value of desc.values) numbers.set(value.jsonName, value.number);
	for (const value of desc.values) numbers.set(value.name, value.number);
	return (json, ignoreUnknownFields) => {
		if (json === null) return zero;
		switch (typeof json) {
			case "number":
				if (Number.isInteger(json)) return json;
				break;
			case "string": {
				const number = numbers.get(json);
				if (number !== void 0) return number;
				if (ignoreUnknownFields) return tokenIgnoredUnknownEnum;
				break;
			}
		}
		throw new Error(`cannot decode ${desc} from JSON: ${formatVal(json)}`);
	};
}
/**
* Compile the check that the reflect API performs for enum values: open
* enums accept any int32 value, closed enums accept only declared values.
*/
function compileEnumCheck(desc) {
	if (desc.open) return checkScalarValue(ScalarType.INT32);
	const values = desc.values;
	return (value) => values.some((v) => v.number === value);
}
/**
* Compile a converter from a JSON value to the local representation of a
* scalar, fusing JSON parsing, the validation of the reflect API, and the
* conversion to the local 64-bit integer representation.
*/
function compileScalarConverter(field) {
	const parseScalar = compileScalarParse(field);
	const checkValue = checkScalarValue(field.scalar);
	const toLocal = compileScalarToLocal(field);
	return (json) => {
		const value = parseScalar(json);
		const check = checkValue(value);
		if (check !== true) throw new FieldError(field, reasonSingular(field, value, check));
		return toLocal(value);
	};
}
/**
* Compile the JSON-specific parsing step for a scalar value: the special
* string values of float and double, string-encoded numbers, and base64
* bytes. Returns the input unchanged if the JSON value cannot be converted;
* the validation step raises an error for it.
*/
function compileScalarParse(field) {
	switch (field.scalar) {
		case ScalarType.DOUBLE:
		case ScalarType.FLOAT: return (json) => {
			if (json === "NaN") return NaN;
			if (json === "Infinity") return Number.POSITIVE_INFINITY;
			if (json === "-Infinity") return Number.NEGATIVE_INFINITY;
			if (typeof json == "number") {
				if (Number.isNaN(json)) throw new FieldError(field, "unexpected NaN number");
				if (!Number.isFinite(json)) throw new FieldError(field, "unexpected infinite number");
				return json;
			}
			if (typeof json == "string") {
				if (json === "") return json;
				if (json.trim().length !== json.length) return json;
				const float = Number(json);
				if (!Number.isFinite(float)) return json;
				return float;
			}
			return json;
		};
		case ScalarType.INT32:
		case ScalarType.FIXED32:
		case ScalarType.SFIXED32:
		case ScalarType.SINT32:
		case ScalarType.UINT32: return int32FromJson;
		case ScalarType.BYTES: return (json) => {
			if (typeof json == "string") {
				if (json === "") return /* @__PURE__ */ new Uint8Array(0);
				try {
					return base64Decode(json);
				} catch (e) {
					throw new FieldError(field, e instanceof Error ? e.message : String(e));
				}
			}
			return json;
		};
		default: return (json) => json;
	}
}
/**
* Compile the conversion of a validated scalar value to its local
* representation: 64-bit integers become bigint, or string with the
* longAsString option.
*/
function compileScalarToLocal(field) {
	const longAsString = field.fieldKind !== "map" && field.longAsString;
	switch (field.scalar) {
		case ScalarType.INT64:
		case ScalarType.SFIXED64:
		case ScalarType.SINT64:
			if (longAsString) return (value) => String(value);
			return (value) => typeof value == "string" || typeof value == "number" ? protoInt64.parse(value) : value;
		case ScalarType.FIXED64:
		case ScalarType.UINT64:
			if (longAsString) return (value) => String(value);
			return (value) => typeof value == "string" || typeof value == "number" ? protoInt64.uParse(value) : value;
		default: return (value) => value;
	}
}
/**
* Return a parser from a JSON value to a map key for the given key type.
* Canonicalizes 64-bit integers given as string, so that "01" and "1" are
* one key, and duplicates can raise an error.
* The parser returns the input if the JSON value cannot be converted.
*/
function compileMapKeyParse(type) {
	switch (type) {
		case ScalarType.BOOL: return (jsonString) => {
			switch (jsonString) {
				case "true": return true;
				case "false": return false;
			}
			return jsonString;
		};
		case ScalarType.INT32:
		case ScalarType.FIXED32:
		case ScalarType.UINT32:
		case ScalarType.SFIXED32:
		case ScalarType.SINT32: return int32FromJson;
		case ScalarType.INT64:
		case ScalarType.SINT64:
		case ScalarType.SFIXED64:
		case ScalarType.UINT64:
		case ScalarType.FIXED64: return (jsonString) => /^-?0+$/.test(jsonString) ? "0" : jsonString.replace(/^(-?)0+(?=\d)/, "$1");
		default: return (jsonString) => jsonString;
	}
}
/**
* Try to parse a JSON value to a 32-bit integer for the reflect API.
*
* Returns the input if the JSON value cannot be converted.
*/
function int32FromJson(json) {
	if (typeof json == "string") {
		if (json === "") return json;
		if (json.trim().length !== json.length) return json;
		const num = Number(json);
		if (Number.isNaN(num)) return json;
		return num;
	}
	return json;
}
/**
* Parse a JSON string, rejecting duplicate object keys (which JSON.parse would
* otherwise silently merge).
*/
function parseJsonString(jsonString, typeName) {
	let json;
	try {
		json = JSON.parse(jsonString);
	} catch (e) {
		const message = e instanceof Error ? e.message : String(e);
		throw new Error(`cannot decode message ${typeName} from JSON: ${message}`, { cause: e });
	}
	checkDuplicateKeys(jsonString, typeName);
	return json;
}
/**
* Scan a JSON string for duplicate object member names at any depth, throwing
* if any are found. JSON.parse() silently keeps the last of duplicate keys, so
* this raw-string scan is the only way to reject them. It must only be called
* with a string that JSON.parse() has already accepted, so it can assume the
* input is well-formed.
*/
function checkDuplicateKeys(jsonString, typeName) {
	const stack = [];
	let expectKey = false;
	let i = 0;
	while (i < jsonString.length) switch (jsonString[i]) {
		case "{":
			stack.push(/* @__PURE__ */ new Set());
			expectKey = true;
			i++;
			break;
		case "[":
			stack.push(null);
			expectKey = false;
			i++;
			break;
		case "}":
		case "]":
			stack.pop();
			expectKey = false;
			i++;
			break;
		case ",":
			expectKey = stack[stack.length - 1] != null;
			i++;
			break;
		case ":":
			expectKey = false;
			i++;
			break;
		case "\"": {
			const open = i++;
			let escaped = false;
			while (i < jsonString.length) {
				if (jsonString[i] == "\\") {
					escaped = true;
					i += 2;
					continue;
				}
				if (jsonString[i] == "\"") break;
				i++;
			}
			const close = i++;
			const seen = stack[stack.length - 1];
			if (expectKey && seen) {
				const name = escaped ? JSON.parse(jsonString.substring(open, close + 1)) : jsonString.substring(open + 1, close);
				if (seen.has(name)) throw new Error(`cannot decode message ${typeName} from JSON: duplicate object key "${name}"`);
				seen.add(name);
			}
			expectKey = false;
			break;
		}
		default: i++;
	}
}
function anyFromJson(any, json, ctx) {
	var _a;
	if (json === null || Array.isArray(json) || typeof json != "object") throw new Error(`cannot decode message ${any.$typeName} from JSON: expected object but got ${formatVal(json)}`);
	if (Object.keys(json).length == 0) return;
	const typeUrl = json["@type"];
	if (typeof typeUrl != "string" || typeUrl == "") throw new Error(`cannot decode message ${any.$typeName} from JSON: "@type" is empty`);
	const typeName = typeUrl.includes("/") ? typeUrl.substring(typeUrl.lastIndexOf("/") + 1) : typeUrl;
	if (!typeName.length) throw new Error(`cannot decode message ${any.$typeName} from JSON: "@type" is invalid`);
	const desc = (_a = ctx.registry) === null || _a === void 0 ? void 0 : _a.getMessage(typeName);
	if (!desc) throw new Error(`cannot decode message ${any.$typeName} from JSON: ${typeUrl} is not in the type registry`);
	const message = create$1(desc);
	if (hasCustomJsonRepresentation(desc) && Object.prototype.hasOwnProperty.call(json, "value")) compiledReader(desc)(message, json.value, ctx);
	else {
		const copy = {};
		for (const key of Object.keys(json)) if (key !== "@type") setOwn(copy, key, json[key]);
		compiledReader(desc)(message, copy, ctx);
	}
	anyPack(desc, message, any);
}
function timestampFromJson(timestamp, json) {
	if (typeof json !== "string") throw new Error(`cannot decode message ${timestamp.$typeName} from JSON: ${formatVal(json)}`);
	const matches = json.match(/^([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2})(?:\.([0-9]{1,9}))?(?:Z|([+-][0-9][0-9]:[0-9][0-9]))$/);
	if (!matches) throw new Error(`cannot decode message ${timestamp.$typeName} from JSON: invalid RFC 3339 string`);
	const ms = Date.parse(matches[1] + "-" + matches[2] + "-" + matches[3] + "T" + matches[4] + ":" + matches[5] + ":" + matches[6] + (matches[8] ? matches[8] : "Z"));
	if (Number.isNaN(ms)) throw new Error(`cannot decode message ${timestamp.$typeName} from JSON: invalid RFC 3339 string`);
	if (ms < timestampMsMin || ms > timestampMsMax) throw new Error(`cannot decode message ${timestamp.$typeName} from JSON: must be from 0001-01-01T00:00:00Z to 9999-12-31T23:59:59Z inclusive`);
	timestamp.seconds = protoInt64.parse(ms / 1e3);
	timestamp.nanos = 0;
	if (matches[7]) timestamp.nanos = parseInt("1" + matches[7] + "0".repeat(9 - matches[7].length)) - 1e9;
}
function durationFromJson(duration, json) {
	if (typeof json !== "string") throw new Error(`cannot decode message ${duration.$typeName} from JSON: ${formatVal(json)}`);
	const match = json.match(/^(-?[0-9]+)(?:\.([0-9]+))?s/);
	if (match === null) throw new Error(`cannot decode message ${duration.$typeName} from JSON: ${formatVal(json)}`);
	const longSeconds = Number(match[1]);
	if (longSeconds > 315576e6 || longSeconds < -315576e6) throw new Error(`cannot decode message ${duration.$typeName} from JSON: ${formatVal(json)}`);
	duration.seconds = protoInt64.parse(longSeconds);
	if (typeof match[2] !== "string") return;
	const nanosStr = match[2] + "0".repeat(9 - match[2].length);
	duration.nanos = parseInt(nanosStr);
	if (longSeconds < 0 || Object.is(longSeconds, -0)) duration.nanos = -duration.nanos;
}
function fieldMaskFromJson(fieldMask, json) {
	if (typeof json !== "string") throw new Error(`cannot decode message ${fieldMask.$typeName} from JSON: ${formatVal(json)}`);
	if (json === "") return;
	fieldMask.paths = json.split(",").map((path) => {
		if (path.includes("_")) throw new Error(`cannot decode message ${fieldMask.$typeName} from JSON: path names must be lowerCamelCase`);
		return protoSnakeCase(path);
	});
}
function structFromJson(struct, json, ctx) {
	if (typeof json != "object" || json == null || Array.isArray(json)) throw new Error(`cannot decode message ${struct.$typeName} from JSON ${formatVal(json)}`);
	const keys = Object.keys(json);
	for (let i = 0; i < keys.length; i++) {
		const key = keys[i];
		const parsedValue = create$1(ValueSchema);
		valueFromJson(parsedValue, json[key], ctx);
		setOwn(struct.fields, key, parsedValue);
	}
}
function valueFromJson(value, json, ctx) {
	if (++ctx.depth > ctx.recursionLimit) throw new Error(`cannot decode ${value.$typeName} from JSON: maximum recursion depth of ${ctx.recursionLimit} reached`);
	switch (typeof json) {
		case "number":
			value.kind = {
				case: "numberValue",
				value: json
			};
			break;
		case "string":
			value.kind = {
				case: "stringValue",
				value: json
			};
			break;
		case "boolean":
			value.kind = {
				case: "boolValue",
				value: json
			};
			break;
		case "object":
			if (json === null) value.kind = {
				case: "nullValue",
				value: NullValue.NULL_VALUE
			};
			else if (Array.isArray(json)) {
				const listValue = create$1(ListValueSchema);
				listValueFromJson(listValue, json, ctx);
				value.kind = {
					case: "listValue",
					value: listValue
				};
			} else {
				const struct = create$1(StructSchema);
				structFromJson(struct, json, ctx);
				value.kind = {
					case: "structValue",
					value: struct
				};
			}
			break;
		default: throw new Error(`cannot decode message ${value.$typeName} from JSON ${formatVal(json)}`);
	}
	ctx.depth--;
	return value;
}
function listValueFromJson(listValue, json, ctx) {
	if (!Array.isArray(json)) throw new Error(`cannot decode message ${listValue.$typeName} from JSON ${formatVal(json)}`);
	for (let i = 0; i < json.length; i++) {
		const value = create$1(ValueSchema);
		valueFromJson(value, json[i], ctx);
		listValue.values.push(value);
	}
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol-connect/code-string.js
/**
* codeToString returns the string representation of a Code.
*
* @private Internal code, does not follow semantic versioning.
*/
function codeToString(value) {
	const name = Code[value];
	if (typeof name != "string") return value.toString();
	return name[0].toLowerCase() + name.substring(1).replace(/[A-Z]/g, (c) => "_" + c.toLowerCase());
}
let stringToCode;
/**
* codeFromString parses the string representation of a Code in snake_case.
* For example, the string "permission_denied" parses into Code.PermissionDenied.
*
* If the given string cannot be parsed, the function returns undefined.
*
* @private Internal code, does not follow semantic versioning.
*/
function codeFromString(value) {
	if (!stringToCode) {
		stringToCode = {};
		for (const value of Object.values(Code)) {
			if (typeof value == "string") continue;
			stringToCode[codeToString(value)] = value;
		}
	}
	return stringToCode[value];
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/connect-error.js
/**
* ConnectError captures four pieces of information: a Code, an error
* message, an optional cause of the error, and an optional collection of
* arbitrary Protobuf messages called  "details".
*
* Because developer tools typically show just the error message, we prefix
* it with the status code, so that the most important information is always
* visible immediately.
*
* Error details are wrapped with google.protobuf.Any on the wire, so that
* a server or middleware can attach arbitrary data to an error. Use the
* method findDetails() to retrieve the details.
*/
var ConnectError = class ConnectError extends Error {
	/**
	* Create a new ConnectError.
	* If no code is provided, code "unknown" is used.
	* Outgoing details are only relevant for the server side - a service may
	* raise an error with details, and it is up to the protocol implementation
	* to encode and send the details along with the error.
	*/
	constructor(message, code = Code.Unknown, metadata, outgoingDetails, cause) {
		super(createMessage(message, code));
		this.name = "ConnectError";
		/**
		* @private Internal field, indicates whether this error was parsed from the
		* wire by a Connect client, as opposed to being created by application code.
		*
		* Do not set this field in application code.
		*/
		this.isWireError = false;
		Object.setPrototypeOf(this, new.target.prototype);
		this.rawMessage = message;
		this.code = code;
		this.metadata = new Headers(metadata !== null && metadata !== void 0 ? metadata : {});
		this.details = outgoingDetails !== null && outgoingDetails !== void 0 ? outgoingDetails : [];
		this.cause = cause;
	}
	/**
	* Convert any value - typically a caught error into a ConnectError,
	* following these rules:
	* - If the value is already a ConnectError, return it as is.
	* - If the value is an AbortError or TimeoutError from the fetch API, return
	*   the message of the error with code Canceled.
	* - For other Errors, return the error message with code Unknown by default.
	* - For other values, return the values String representation as a message,
	*   with the code Unknown by default.
	* The original value will be used for the "cause" property for the new
	* ConnectError.
	*/
	static from(reason, code = Code.Unknown) {
		if (reason instanceof ConnectError) return reason;
		if (reason instanceof Error) {
			if (reason.name == "AbortError" || reason.name == "TimeoutError") return new ConnectError(reason.message, Code.Canceled);
			return new ConnectError(reason.message, code, void 0, void 0, reason);
		}
		return new ConnectError(String(reason), code, void 0, void 0, reason);
	}
	static [Symbol.hasInstance](v) {
		if (!(v instanceof Error)) return false;
		if (Object.getPrototypeOf(v) === ConnectError.prototype) return true;
		return v.name === "ConnectError" && "code" in v && typeof v.code === "number" && "metadata" in v && "details" in v && Array.isArray(v.details) && "rawMessage" in v && typeof v.rawMessage == "string" && "cause" in v;
	}
	findDetails(typeOrRegistry) {
		const registry = typeOrRegistry.kind === "message" ? { getMessage: (typeName) => typeName === typeOrRegistry.typeName ? typeOrRegistry : void 0 } : typeOrRegistry;
		const details = [];
		for (const data of this.details) {
			if ("desc" in data) {
				if (registry.getMessage(data.desc.typeName)) details.push(create$1(data.desc, data.value));
				continue;
			}
			const desc = registry.getMessage(data.type);
			if (desc) try {
				details.push(fromBinary(desc, data.value));
			} catch (_) {}
		}
		return details;
	}
};
/**
* Create an error message, prefixing the given code.
*/
function createMessage(message, code) {
	return message.length ? `[${codeToString(code)}] ${message}` : `[${codeToString(code)}]`;
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/http-headers.js
/**
* Merge two or more Headers objects by appending all fields from
* all inputs to a new Headers object.
*/
function appendHeaders(...headers) {
	const h = new Headers();
	for (const e of headers) e.forEach((value, key) => {
		h.append(key, value);
	});
	return h;
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/any-client.js
/**
* Create any client for the given service.
*
* The given createMethod function is called for each method definition
* of the service. The function it returns is added to the client object
* as a method.
*/
function makeAnyClient(service, createMethod) {
	const client = {};
	for (const desc of service.methods) {
		const method = createMethod(desc);
		if (method != null) client[desc.localName] = method;
	}
	return client;
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol/limit-io.js
/**
* Raise an error ResourceExhausted if more than readMaxBytes are read.
*
* @private Internal code, does not follow semantic versioning.
*/
function assertReadMaxBytes(readMaxBytes, bytesRead, totalSizeKnown = false) {
	if (bytesRead > readMaxBytes) {
		let message = `message size is larger than configured readMaxBytes ${readMaxBytes}`;
		if (totalSizeKnown) message = `message size ${bytesRead} is larger than configured readMaxBytes ${readMaxBytes}`;
		throw new ConnectError(message, Code.ResourceExhausted);
	}
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol/envelope.js
/**
* Create an EnvelopeDecoder. The `readMaxBytes` argument limits the maximum
* size for individual messages.
*
* @private Internal code, does not follow semantic versioning.
*/
function createEnvelopeDecoder(readMaxBytes) {
	return new EnvelopeDecoderImpl(readMaxBytes);
}
var EnvelopeDecoderImpl = class {
	constructor(readMaxBytes) {
		this.readMaxBytes = readMaxBytes;
		this.header = /* @__PURE__ */ new Uint8Array(5);
		this.headerView = new DataView(this.header.buffer);
		this.buf = [];
	}
	get byteLength() {
		return this.buf.reduce((a, b) => a + b.byteLength, 0);
	}
	decode(chunk) {
		this.buf.push(chunk);
		const envs = [];
		for (;;) {
			let env = this.pop();
			if (!env) break;
			envs.push(env);
		}
		return envs;
	}
	pop() {
		if (!this.env) {
			this.env = this.head();
			if (!this.env) return;
		}
		if (this.cons(this.env.data)) {
			const env = this.env;
			this.env = void 0;
			return env;
		}
	}
	head() {
		if (!this.cons(this.header)) return;
		const flags = this.headerView.getUint8(0);
		const length = this.headerView.getUint32(1);
		assertReadMaxBytes(this.readMaxBytes, length, true);
		return {
			flags,
			data: new Uint8Array(length)
		};
	}
	cons(target) {
		const wantLength = target.byteLength;
		if (this.byteLength < wantLength) return false;
		let offset = 0;
		while (offset < wantLength) {
			const chunk = this.buf.shift();
			if (chunk.byteLength > wantLength - offset) {
				target.set(chunk.subarray(0, wantLength - offset), offset);
				this.buf.unshift(chunk.subarray(wantLength - offset));
				offset += wantLength - offset;
			} else {
				target.set(chunk, offset);
				offset += chunk.byteLength;
			}
		}
		return true;
	}
};
/**
* Create a WHATWG ReadableStream of enveloped messages from a ReadableStream
* of bytes.
*
* Ideally, this would simply be a TransformStream, but ReadableStream.pipeThrough
* does not have the necessary availability at this time.
*
* @private Internal code, does not follow semantic versioning.
*/
function createEnvelopeReadableStream(stream) {
	let reader;
	const buffer = createEnvelopeDecoder(4294967295);
	return new ReadableStream({
		start() {
			reader = stream.getReader();
		},
		async pull(controller) {
			let enqueuedOnce = false;
			while (!enqueuedOnce) {
				const result = await reader.read();
				if (result.done) {
					if (buffer.byteLength > 0) controller.error(new ConnectError("protocol error: incomplete envelope", Code.InvalidArgument));
					controller.close();
				} else for (const env of buffer.decode(result.value)) {
					controller.enqueue(env);
					enqueuedOnce = true;
				}
			}
		}
	});
}
/**
* Encode a single enveloped message.
*
* @private Internal code, does not follow semantic versioning.
*/
function encodeEnvelope(flags, data) {
	const bytes = new Uint8Array(data.length + 5);
	bytes.set(data, 5);
	const v = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
	v.setUint8(0, flags);
	v.setUint32(1, data.length);
	return bytes;
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol/async-iterable.js
var __asyncValues$1 = function(o) {
	if (!Symbol.asyncIterator) throw new TypeError("Symbol.asyncIterator is not defined.");
	var m = o[Symbol.asyncIterator], i;
	return m ? m.call(o) : (o = typeof __values === "function" ? __values(o) : o[Symbol.iterator](), i = {}, verb("next"), verb("throw"), verb("return"), i[Symbol.asyncIterator] = function() {
		return this;
	}, i);
	function verb(n) {
		i[n] = o[n] && function(v) {
			return new Promise(function(resolve, reject) {
				v = o[n](v), settle(resolve, reject, v.done, v.value);
			});
		};
	}
	function settle(resolve, reject, d, v) {
		Promise.resolve(v).then(function(v) {
			resolve({
				value: v,
				done: d
			});
		}, reject);
	}
};
var __await$2 = function(v) {
	return this instanceof __await$2 ? (this.v = v, this) : new __await$2(v);
};
var __asyncGenerator$2 = function(thisArg, _arguments, generator) {
	if (!Symbol.asyncIterator) throw new TypeError("Symbol.asyncIterator is not defined.");
	var g = generator.apply(thisArg, _arguments || []), i, q = [];
	return i = Object.create((typeof AsyncIterator === "function" ? AsyncIterator : Object).prototype), verb("next"), verb("throw"), verb("return", awaitReturn), i[Symbol.asyncIterator] = function() {
		return this;
	}, i;
	function awaitReturn(f) {
		return function(v) {
			return Promise.resolve(v).then(f, reject);
		};
	}
	function verb(n, f) {
		if (g[n]) {
			i[n] = function(v) {
				return new Promise(function(a, b) {
					q.push([
						n,
						v,
						a,
						b
					]) > 1 || resume(n, v);
				});
			};
			if (f) i[n] = f(i[n]);
		}
	}
	function resume(n, v) {
		try {
			step(g[n](v));
		} catch (e) {
			settle(q[0][3], e);
		}
	}
	function step(r) {
		r.value instanceof __await$2 ? Promise.resolve(r.value.v).then(fulfill, reject) : settle(q[0][2], r);
	}
	function fulfill(value) {
		resume("next", value);
	}
	function reject(value) {
		resume("throw", value);
	}
	function settle(f, v) {
		if (f(v), q.shift(), q.length) resume(q[0][0], q[0][1]);
	}
};
var __asyncDelegator$1 = function(o) {
	var i = {}, p;
	return verb("next"), verb("throw", function(e) {
		throw e;
	}), verb("return"), i[Symbol.iterator] = function() {
		return this;
	}, i;
	function verb(n, f) {
		i[n] = o[n] ? function(v) {
			return (p = !p) ? {
				value: __await$2(o[n](v)),
				done: false
			} : f ? f(v) : v;
		} : f;
	}
};
/**
* Create an asynchronous iterable from an array.
*
* @private Internal code, does not follow semantic versioning.
*/
function createAsyncIterable(items) {
	return __asyncGenerator$2(this, arguments, function* createAsyncIterable_1() {
		yield __await$2(yield* __asyncDelegator$1(__asyncValues$1(items)));
	});
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/promise-client.js
var __asyncValues = function(o) {
	if (!Symbol.asyncIterator) throw new TypeError("Symbol.asyncIterator is not defined.");
	var m = o[Symbol.asyncIterator], i;
	return m ? m.call(o) : (o = typeof __values === "function" ? __values(o) : o[Symbol.iterator](), i = {}, verb("next"), verb("throw"), verb("return"), i[Symbol.asyncIterator] = function() {
		return this;
	}, i);
	function verb(n) {
		i[n] = o[n] && function(v) {
			return new Promise(function(resolve, reject) {
				v = o[n](v), settle(resolve, reject, v.done, v.value);
			});
		};
	}
	function settle(resolve, reject, d, v) {
		Promise.resolve(v).then(function(v) {
			resolve({
				value: v,
				done: d
			});
		}, reject);
	}
};
var __await$1 = function(v) {
	return this instanceof __await$1 ? (this.v = v, this) : new __await$1(v);
};
var __asyncDelegator = function(o) {
	var i = {}, p;
	return verb("next"), verb("throw", function(e) {
		throw e;
	}), verb("return"), i[Symbol.iterator] = function() {
		return this;
	}, i;
	function verb(n, f) {
		i[n] = o[n] ? function(v) {
			return (p = !p) ? {
				value: __await$1(o[n](v)),
				done: false
			} : f ? f(v) : v;
		} : f;
	}
};
var __asyncGenerator$1 = function(thisArg, _arguments, generator) {
	if (!Symbol.asyncIterator) throw new TypeError("Symbol.asyncIterator is not defined.");
	var g = generator.apply(thisArg, _arguments || []), i, q = [];
	return i = Object.create((typeof AsyncIterator === "function" ? AsyncIterator : Object).prototype), verb("next"), verb("throw"), verb("return", awaitReturn), i[Symbol.asyncIterator] = function() {
		return this;
	}, i;
	function awaitReturn(f) {
		return function(v) {
			return Promise.resolve(v).then(f, reject);
		};
	}
	function verb(n, f) {
		if (g[n]) {
			i[n] = function(v) {
				return new Promise(function(a, b) {
					q.push([
						n,
						v,
						a,
						b
					]) > 1 || resume(n, v);
				});
			};
			if (f) i[n] = f(i[n]);
		}
	}
	function resume(n, v) {
		try {
			step(g[n](v));
		} catch (e) {
			settle(q[0][3], e);
		}
	}
	function step(r) {
		r.value instanceof __await$1 ? Promise.resolve(r.value.v).then(fulfill, reject) : settle(q[0][2], r);
	}
	function fulfill(value) {
		resume("next", value);
	}
	function reject(value) {
		resume("throw", value);
	}
	function settle(f, v) {
		if (f(v), q.shift(), q.length) resume(q[0][0], q[0][1]);
	}
};
/**
* Create a Client for the given service, invoking RPCs through the
* given transport.
*/
function createClient(service, transport) {
	return makeAnyClient(service, (method) => {
		switch (method.methodKind) {
			case "unary": return createUnaryFn(transport, method);
			case "server_streaming": return createServerStreamingFn(transport, method);
			case "client_streaming": return createClientStreamingFn(transport, method);
			case "bidi_streaming": return createBiDiStreamingFn(transport, method);
			default: return null;
		}
	});
}
function createUnaryFn(transport, method) {
	return async (input, options) => {
		var _a, _b;
		const response = await transport.unary(method, options === null || options === void 0 ? void 0 : options.signal, options === null || options === void 0 ? void 0 : options.timeoutMs, options === null || options === void 0 ? void 0 : options.headers, input, options === null || options === void 0 ? void 0 : options.contextValues);
		(_a = options === null || options === void 0 ? void 0 : options.onHeader) === null || _a === void 0 || _a.call(options, response.header);
		(_b = options === null || options === void 0 ? void 0 : options.onTrailer) === null || _b === void 0 || _b.call(options, response.trailer);
		return response.message;
	};
}
function createServerStreamingFn(transport, method) {
	return (input, options) => handleStreamResponse(transport.stream(method, options === null || options === void 0 ? void 0 : options.signal, options === null || options === void 0 ? void 0 : options.timeoutMs, options === null || options === void 0 ? void 0 : options.headers, createAsyncIterable([input]), options === null || options === void 0 ? void 0 : options.contextValues), options);
}
function createClientStreamingFn(transport, method) {
	return async (request, options) => {
		var _a, e_1, _b, _c;
		var _d, _e;
		const response = await transport.stream(method, options === null || options === void 0 ? void 0 : options.signal, options === null || options === void 0 ? void 0 : options.timeoutMs, options === null || options === void 0 ? void 0 : options.headers, request, options === null || options === void 0 ? void 0 : options.contextValues);
		(_d = options === null || options === void 0 ? void 0 : options.onHeader) === null || _d === void 0 || _d.call(options, response.header);
		let singleMessage;
		let count = 0;
		try {
			for (var _f = true, _g = __asyncValues(response.message), _h; _h = await _g.next(), _a = _h.done, !_a; _f = true) {
				_c = _h.value;
				_f = false;
				singleMessage = _c;
				count++;
			}
		} catch (e_1_1) {
			e_1 = { error: e_1_1 };
		} finally {
			try {
				if (!_f && !_a && (_b = _g.return)) await _b.call(_g);
			} finally {
				if (e_1) throw e_1.error;
			}
		}
		if (!singleMessage) throw new ConnectError("protocol error: missing response message", Code.Unimplemented);
		if (count > 1) throw new ConnectError("protocol error: received extra messages for client streaming method", Code.Unimplemented);
		(_e = options === null || options === void 0 ? void 0 : options.onTrailer) === null || _e === void 0 || _e.call(options, response.trailer);
		return singleMessage;
	};
}
function createBiDiStreamingFn(transport, method) {
	return (request, options) => handleStreamResponse(transport.stream(method, options === null || options === void 0 ? void 0 : options.signal, options === null || options === void 0 ? void 0 : options.timeoutMs, options === null || options === void 0 ? void 0 : options.headers, request, options === null || options === void 0 ? void 0 : options.contextValues), options);
}
function handleStreamResponse(stream, options) {
	return (function() {
		return __asyncGenerator$1(this, arguments, function* () {
			var _a, _b;
			const response = yield __await$1(stream);
			(_a = options === null || options === void 0 ? void 0 : options.onHeader) === null || _a === void 0 || _a.call(options, response.header);
			yield __await$1(yield* __asyncDelegator(__asyncValues(response.message)));
			(_b = options === null || options === void 0 ? void 0 : options.onTrailer) === null || _b === void 0 || _b.call(options, response.trailer);
		});
	})();
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol/signals.js
/**
* Create an AbortController that is automatically aborted if one of the given
* signals is aborted.
*
* For convenience, the linked AbortSignals can be undefined.
*
* If the controller or any of the signals is aborted, all event listeners are
* removed.
*
* @private Internal code, does not follow semantic versioning.
*/
function createLinkedAbortController(...signals) {
	const controller = new AbortController();
	const sa = signals.filter((s) => s !== void 0).concat(controller.signal);
	for (const signal of sa) {
		if (signal.aborted) {
			onAbort.apply(signal);
			break;
		}
		signal.addEventListener("abort", onAbort);
	}
	function onAbort() {
		if (!controller.signal.aborted) controller.abort(getAbortSignalReason(this));
		for (const signal of sa) signal.removeEventListener("abort", onAbort);
	}
	return controller;
}
/**
* Create a deadline signal. The returned object contains an AbortSignal, but
* also a cleanup function to stop the timer, which must be called once the
* calling code is no longer interested in the signal.
*
* Ideally, we would simply use AbortSignal.timeout(), but it is not widely
* available yet.
*
* @private Internal code, does not follow semantic versioning.
*/
function createDeadlineSignal(timeoutMs) {
	const controller = new AbortController();
	const listener = () => {
		controller.abort(new ConnectError("the operation timed out", Code.DeadlineExceeded));
	};
	let timeoutId;
	if (timeoutMs !== void 0) {
		if (timeoutMs <= 0) listener();
		else timeoutId = setTimeout(listener, timeoutMs);
	}
	return {
		signal: controller.signal,
		cleanup: () => clearTimeout(timeoutId)
	};
}
/**
* Returns the reason why an AbortSignal was aborted. Returns undefined if the
* signal has not been aborted.
*
* The property AbortSignal.reason is not widely available. This function
* returns an AbortError if the signal is aborted, but reason is undefined.
*
* @private Internal code, does not follow semantic versioning.
*/
function getAbortSignalReason(signal) {
	if (!signal.aborted) return;
	if (signal.reason !== void 0) return signal.reason;
	const e = /* @__PURE__ */ new Error("This operation was aborted");
	e.name = "AbortError";
	return e;
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/context-values.js
/**
* createContextValues creates a new ContextValues.
*/
function createContextValues() {
	return {
		get(key) {
			return key.id in this ? this[key.id] : key.defaultValue;
		},
		set(key, value) {
			this[key.id] = value;
			return this;
		},
		delete(key) {
			delete this[key.id];
			return this;
		}
	};
}
// ../../node_modules/.pnpm/@bufbuild+protobuf@2.16.0/node_modules/@bufbuild/protobuf/dist/esm/codegenv2/service.js
/**
* Hydrate a service descriptor.
*
* @private
*/
function serviceDesc(file, path, ...paths) {
	if (paths.length > 0) throw new Error();
	return file.services[path];
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol-connect/headers.js
/**
* @private Internal code, does not follow semantic versioning.
*/
const headerContentType = "Content-Type";
const headerUnaryContentLength = "Content-Length";
const headerUnaryEncoding = "Content-Encoding";
const headerUnaryAcceptEncoding = "Accept-Encoding";
const headerTimeout = "Connect-Timeout-Ms";
const headerProtocolVersion = "Connect-Protocol-Version";
const headerUserAgent = "User-Agent";
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol/create-method-url.js
/**
* Create a URL for the given RPC. This simply adds the qualified
* service name, a slash, and the method name to the path of the given
* baseUrl.
*
* For example, the baseUri https://example.com and method "Say" from
* the service example.ElizaService results in:
* https://example.com/example.ElizaService/Say
*
* This format is used by the protocols Connect, gRPC and Twirp.
*
* Note that this function also accepts a protocol-relative baseUrl.
* If given an empty string or "/" as a baseUrl, it returns just the
* path.
*/
function createMethodUrl(baseUrl, method) {
	return baseUrl.toString().replace(/\/?$/, `/${method.parent.typeName}/${method.name}`);
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol/normalize.js
/**
*  Takes a partial protobuf messages of the
*  specified message type as input, and returns full instances.
*/
function normalize(desc, message) {
	return create$1(desc, message);
}
/**
* Takes an AsyncIterable of partial protobuf messages of the
* specified message type as input, and yields full instances.
*/
function normalizeIterable(desc, input) {
	function transform(result) {
		if (result.done === true) return result;
		return {
			done: result.done,
			value: normalize(desc, result.value)
		};
	}
	return { [Symbol.asyncIterator]() {
		const it = input[Symbol.asyncIterator]();
		const res = { next: () => it.next().then(transform) };
		if (it.throw !== void 0) res.throw = (e) => it.throw(e).then(transform);
		if (it.return !== void 0) res.return = (v) => it.return(v).then(transform);
		return res;
	} };
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/interceptor.js
/**
* applyInterceptors takes the given UnaryFn or ServerStreamingFn, and wraps
* it with each of the given interceptors, returning a new UnaryFn or
* ServerStreamingFn.
*/
function applyInterceptors(next, interceptors) {
	if (!interceptors) return next;
	for (const i of interceptors.concat().reverse()) next = i(next);
	return next;
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol/serialization.js
/**
* Sets default JSON serialization options for connect-es.
*
* With standard protobuf JSON serialization, unknown JSON fields are
* rejected by default. In connect-es, unknown JSON fields are ignored
* by default.
*/
function getJsonOptions(options) {
	var _a;
	const o = Object.assign({}, options);
	(_a = o.ignoreUnknownFields) !== null && _a !== void 0 || (o.ignoreUnknownFields = true);
	return o;
}
/**
* Returns functions to normalize and serialize the input message
* of an RPC, and to parse the output message of an RPC.
*
* @private Internal code, does not follow semantic versioning.
*/
function createClientMethodSerializers(method, useBinaryFormat, jsonOptions, binaryOptions) {
	const input = useBinaryFormat ? createBinarySerialization(method.input, binaryOptions) : createJsonSerialization(method.input, jsonOptions);
	return {
		parse: (useBinaryFormat ? createBinarySerialization(method.output, binaryOptions) : createJsonSerialization(method.output, jsonOptions)).parse,
		serialize: input.serialize
	};
}
/**
* Creates a Serialization object for serializing the given protobuf message
* with the protobuf binary format.
*/
function createBinarySerialization(desc, options) {
	return {
		parse(data) {
			try {
				return fromBinary(desc, data, options);
			} catch (e) {
				throw new ConnectError(`parse binary: ${e instanceof Error ? e.message : String(e)}`, Code.Internal);
			}
		},
		serialize(data) {
			try {
				return toBinary(desc, data, options);
			} catch (e) {
				throw new ConnectError(`serialize binary: ${e instanceof Error ? e.message : String(e)}`, Code.Internal);
			}
		}
	};
}
/**
* Creates a Serialization object for serializing the given protobuf message
* with the protobuf canonical JSON encoding.
*
* By default, unknown fields are ignored.
*/
function createJsonSerialization(desc, options) {
	var _a, _b;
	const textEncoder = (_a = options === null || options === void 0 ? void 0 : options.textEncoder) !== null && _a !== void 0 ? _a : new TextEncoder();
	const textDecoder = (_b = options === null || options === void 0 ? void 0 : options.textDecoder) !== null && _b !== void 0 ? _b : new TextDecoder();
	const o = getJsonOptions(options);
	return {
		parse(data) {
			try {
				return fromJsonString(desc, textDecoder.decode(data), o);
			} catch (e) {
				throw ConnectError.from(e, Code.InvalidArgument);
			}
		},
		serialize(data) {
			try {
				const json = toJsonString(desc, data, o);
				return textEncoder.encode(json);
			} catch (e) {
				throw ConnectError.from(e, Code.Internal);
			}
		}
	};
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol-connect/content-type.js
/**
* Regular Expression that matches any valid Connect Content-Type header value.
*
* @private Internal code, does not follow semantic versioning.
*/
const contentTypeRegExp = /^application\/(connect\+)?(?:(json)(?:; ?charset=utf-?8)?|(proto))$/i;
const contentTypeUnaryProto = "application/proto";
const contentTypeUnaryJson = "application/json";
const contentTypeStreamProto = "application/connect+proto";
const contentTypeStreamJson = "application/connect+json";
/**
* Parse a Connect Content-Type header.
*
* @private Internal code, does not follow semantic versioning.
*/
function parseContentType(contentType) {
	const match = contentType === null || contentType === void 0 ? void 0 : contentType.match(contentTypeRegExp);
	if (!match) return;
	return {
		stream: !!match[1],
		binary: !!match[3]
	};
}
/**
* Parse a Connect error from a JSON value.
* Will return a ConnectError, and throw the provided fallback if parsing failed.
*
* @private Internal code, does not follow semantic versioning.
*/
function errorFromJson(jsonValue, metadata, fallback) {
	var _a;
	if (metadata) new Headers(metadata).forEach((value, key) => fallback.metadata.append(key, value));
	if (typeof jsonValue !== "object" || jsonValue == null || Array.isArray(jsonValue)) throw fallback;
	let code = fallback.code;
	if ("code" in jsonValue && typeof jsonValue.code === "string") code = (_a = codeFromString(jsonValue.code)) !== null && _a !== void 0 ? _a : code;
	const message = jsonValue.message;
	if (message != null && typeof message !== "string") throw fallback;
	const error = new ConnectError(message !== null && message !== void 0 ? message : "", code, metadata);
	error.isWireError = true;
	if ("details" in jsonValue && Array.isArray(jsonValue.details)) for (const detail of jsonValue.details) {
		if (detail === null || typeof detail != "object" || Array.isArray(detail) || typeof detail.type != "string" || typeof detail.value != "string") throw fallback;
		try {
			error.details.push({
				type: detail.type,
				value: base64Decode(detail.value),
				debug: detail.debug
			});
		} catch (e) {
			throw fallback;
		}
	}
	return error;
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol-connect/end-stream.js
/**
* Parse an EndStreamResponse of the Connect protocol.
* Throws a ConnectError on malformed input.
*
* @private Internal code, does not follow semantic versioning.
*/
function endStreamFromJson(data) {
	const parseErr = new ConnectError("invalid end stream", Code.Unknown);
	let jsonValue;
	try {
		jsonValue = JSON.parse(typeof data == "string" ? data : new TextDecoder().decode(data));
	} catch (e) {
		throw parseErr;
	}
	if (typeof jsonValue != "object" || jsonValue == null || Array.isArray(jsonValue)) throw parseErr;
	const metadata = new Headers();
	if ("metadata" in jsonValue) {
		if (typeof jsonValue.metadata != "object" || jsonValue.metadata == null || Array.isArray(jsonValue.metadata)) throw parseErr;
		for (const [key, values] of Object.entries(jsonValue.metadata)) {
			if (!Array.isArray(values) || values.some((value) => typeof value != "string")) throw parseErr;
			for (const value of values) metadata.append(key, value);
		}
	}
	return {
		metadata,
		error: "error" in jsonValue && jsonValue.error != null ? errorFromJson(jsonValue.error, metadata, parseErr) : void 0
	};
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol-connect/http-status.js
/**
* Determine the Connect error code for the given HTTP status code.
* See https://connectrpc.com/docs/protocol/#http-to-error-code
*
* @private Internal code, does not follow semantic versioning.
*/
function codeFromHttpStatus(httpStatus) {
	switch (httpStatus) {
		case 400: return Code.Internal;
		case 401: return Code.Unauthenticated;
		case 403: return Code.PermissionDenied;
		case 404: return Code.Unimplemented;
		case 429: return Code.Unavailable;
		case 502: return Code.Unavailable;
		case 503: return Code.Unavailable;
		case 504: return Code.Unavailable;
		default: return Code.Unknown;
	}
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol-connect/trailer-mux.js
/**
* In unary RPCs, Connect transports trailing metadata as response header
* fields, prefixed with "trailer-".
*
* This function demuxes headers and trailers into two separate Headers
* objects.
*
* @private Internal code, does not follow semantic versioning.
*/
function trailerDemux(header) {
	const h = new Headers(), t = new Headers();
	header.forEach((value, key) => {
		if (key.toLowerCase().startsWith("trailer-")) t.append(key.substring(8), value);
		else h.append(key, value);
	});
	return [h, t];
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol-connect/request-header.js
/**
* Creates headers for a Connect request.
*
* @private Internal code, does not follow semantic versioning.
*/
function requestHeader(methodKind, useBinaryFormat, timeoutMs, userProvidedHeaders, setUserAgent) {
	const result = new Headers(userProvidedHeaders !== null && userProvidedHeaders !== void 0 ? userProvidedHeaders : {});
	if (timeoutMs !== void 0) result.set(headerTimeout, `${timeoutMs}`);
	result.set(headerContentType, methodKind == "unary" ? useBinaryFormat ? contentTypeUnaryProto : contentTypeUnaryJson : useBinaryFormat ? contentTypeStreamProto : contentTypeStreamJson);
	result.set(headerProtocolVersion, "1");
	if (!result.has("User-Agent") && setUserAgent) result.set(headerUserAgent, "connect-es/2.3.0");
	return result;
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol-connect/validate-response.js
/**
* Validates response status and header for the Connect protocol.
* Throws a ConnectError if the header indicates an error, or if
* the content type is unexpected, with the following exception:
* For unary RPCs with an HTTP error status, this returns an error
* derived from the HTTP status instead of throwing it, giving an
* implementation a chance to parse a Connect error from the wire.
*
* @private Internal code, does not follow semantic versioning.
*/
function validateResponse(methodKind, useBinaryFormat, status, headers) {
	const mimeType = headers.get(headerContentType);
	const parsedType = parseContentType(mimeType);
	if (status !== 200) {
		const errorFromStatus = new ConnectError(`HTTP ${status}`, codeFromHttpStatus(status), headers);
		if (methodKind == "unary" && parsedType && !parsedType.binary) return {
			isUnaryError: true,
			unaryError: errorFromStatus
		};
		throw errorFromStatus;
	}
	const allowedContentType = {
		binary: useBinaryFormat,
		stream: methodKind !== "unary"
	};
	if ((parsedType === null || parsedType === void 0 ? void 0 : parsedType.binary) !== allowedContentType.binary || parsedType.stream !== allowedContentType.stream) throw new ConnectError(`unsupported content type ${mimeType}`, parsedType === void 0 ? Code.Unknown : Code.Internal, headers);
	return { isUnaryError: false };
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol-connect/get-request.js
const contentTypePrefix = "application/";
function encodeMessageForUrl(message, useBase64) {
	if (useBase64) return base64Encode(message, "url");
	return encodeURIComponent(new TextDecoder().decode(message));
}
/**
* @private Internal code, does not follow semantic versioning.
*/
function transformConnectPostToGetRequest(request, message, useBase64) {
	let query = `?connect=v1`;
	let compressionQuery = "";
	const compression = request.header.get(headerUnaryEncoding);
	if (compression !== null && compression !== "identity") {
		compressionQuery = "&compression=" + encodeURIComponent(compression);
		useBase64 = true;
	}
	if (useBase64) query += "&base64=1";
	query += compressionQuery;
	const contentType = request.header.get(headerContentType);
	if ((contentType === null || contentType === void 0 ? void 0 : contentType.indexOf(contentTypePrefix)) === 0) query += "&encoding=" + encodeURIComponent(contentType.slice(12));
	query += "&message=" + encodeMessageForUrl(message, useBase64);
	const url = request.url + query;
	const header = new Headers(request.header);
	for (const h of [
		headerProtocolVersion,
		headerContentType,
		headerUnaryContentLength,
		headerUnaryEncoding,
		headerUnaryAcceptEncoding
	]) header.delete(h);
	return Object.assign(Object.assign({}, request), {
		requestMethod: "GET",
		url,
		header
	});
}
// ../../node_modules/.pnpm/@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0/node_modules/@connectrpc/connect/dist/esm/protocol/run-call.js
/**
* Runs a unary method with the given interceptors. Note that this function
* is only used when implementing a Transport.
*/
function runUnaryCall(opt) {
	const next = applyInterceptors(opt.next, opt.interceptors);
	const [signal, abort, done] = setupSignal(opt);
	return next(Object.assign(Object.assign({}, opt.req), {
		message: normalize(opt.req.method.input, opt.req.message),
		signal
	})).then((res) => {
		done();
		return res;
	}, abort);
}
/**
* Runs a server-streaming method with the given interceptors. Note that this
* function is only used when implementing a Transport.
*/
function runStreamingCall(opt) {
	const [signal, abort, done] = setupSignal(opt);
	let doneCalled = false;
	const aborted = new Promise((resolve) => {
		if (signal.aborted) resolve();
		else signal.addEventListener("abort", () => resolve());
	});
	aborted.then(() => {
		var _a, _b;
		const it = opt.req.message[Symbol.asyncIterator]();
		if (!doneCalled) {
			const error = ConnectError.from(getAbortSignalReason(signal), Code.Canceled);
			(_a = it.throw) === null || _a === void 0 || _a.call(it, error).catch(() => {});
		}
		(_b = it.return) === null || _b === void 0 || _b.call(it).catch(() => {});
	});
	return applyInterceptors(async (req) => {
		const res = await opt.next(req);
		const it = res.message[Symbol.asyncIterator]();
		aborted.then(() => {
			var _a;
			return (_a = it.return) === null || _a === void 0 ? void 0 : _a.call(it).catch(() => {});
		});
		return Object.assign(Object.assign({}, res), { message: { [Symbol.asyncIterator]: () => ({ next: async () => {
			const result = signal.aborted ? void 0 : await Promise.race([it.next(), aborted]);
			if (result !== void 0) return result;
			return doneCalled ? {
				done: true,
				value: void 0
			} : abort(getAbortSignalReason(signal));
		} }) } });
	}, opt.interceptors)(Object.assign(Object.assign({}, opt.req), {
		message: normalizeIterable(opt.req.method.input, opt.req.message),
		signal
	})).then((res) => {
		const it = res.message[Symbol.asyncIterator]();
		const drained = aborted.then(async () => {
			try {
				while ((await it.next()).done !== true);
			} catch (_a) {}
		});
		return Object.assign(Object.assign({}, res), { message: { [Symbol.asyncIterator]: () => ({
			async next() {
				if (signal.aborted) {
					await drained;
					return doneCalled ? {
						done: true,
						value: void 0
					} : abort(getAbortSignalReason(signal));
				}
				try {
					const result = await it.next();
					if (result.done === true) {
						doneCalled = true;
						done();
					}
					return result;
				} catch (e) {
					return abort(e);
				}
			},
			async return(value) {
				if (!signal.aborted) {
					doneCalled = true;
					done();
				}
				await drained;
				return {
					done: true,
					value
				};
			},
			async throw(reason) {
				if (doneCalled) throw ConnectError.from(reason);
				try {
					return await abort(reason);
				} finally {
					await drained;
				}
			}
		}) } });
	}, abort);
}
/**
* Create an AbortSignal for Transport implementations. The signal is available
* in UnaryRequest and StreamingRequest, and is triggered when the call is
* aborted (via a timeout or explicit cancellation), errored (e.g. when reading
* an error from the server from the wire), or finished successfully.
*
* Transport implementations can pass the signal to HTTP clients to ensure that
* there are no unused connections leak.
*
* Returns a tuple:
* [0]: The signal, which is also aborted if the optional deadline is reached.
* [1]: Function to call if the Transport encountered an error.
* [2]: Function to call if the Transport finished without an error.
*/
function setupSignal(opt) {
	const { signal, cleanup } = createDeadlineSignal(opt.timeoutMs);
	const controller = createLinkedAbortController(opt.signal, signal);
	return [
		controller.signal,
		function abort(reason) {
			const e = controller.signal.aborted ? ConnectError.from(getAbortSignalReason(controller.signal), Code.Canceled) : ConnectError.from(reason);
			controller.abort(e);
			cleanup();
			return Promise.reject(e);
		},
		function done() {
			cleanup();
			controller.abort();
		}
	];
}
// ../client/dist/agx_pb-DBZL7cO2.mjs
/**
* Describes the file agx/v1/agx.proto.
*/
const file_agx_v1_agx = /*@__PURE__*/ fileDesc("ChBhZ3gvdjEvYWd4LnByb3RvEgZhZ3gudjEiWgoLUmVzb3VyY2VSZWYSEgoIc2tpbGxfaWQYASABKAlIABIZCg9pbnN0cnVjdGlvbnNfaWQYAiABKAlIABIQCgZtY3BfaWQYAyABKAlIAEIKCghyZXNvdXJjZSJLCg5Tb3VyY2VMb2NhdGlvbhIMCgRwYXRoGAEgASgJEgwKBGxpbmUYAiABKA0SDgoGY29sdW1uGAMgASgNEg0KBWxvY2FsGAQgASgIItIBCgpEaWFnbm9zdGljEi0KCHNldmVyaXR5GAEgASgOMhsuYWd4LnYxLkRpYWdub3N0aWMuU2V2ZXJpdHkSDAoEY29kZRgCIAEoCRIPCgdzdW1tYXJ5GAMgASgJEiYKBnNvdXJjZRgEIAEoCzIWLmFneC52MS5Tb3VyY2VMb2NhdGlvbiJOCghTZXZlcml0eRIYChRTRVZFUklUWV9VTlNQRUNJRklFRBAAEhQKEFNFVkVSSVRZX1dBUk5JTkcQARISCg5TRVZFUklUWV9FUlJPUhACIjUKC0Vycm9yRGV0YWlsEiYKBnNvdXJjZRgCIAEoCzIWLmFneC52MS5Tb3VyY2VMb2NhdGlvbiKAAQoIU25hcHNob3QSCgoCaWQYASABKAkSEQoJZGlyZWN0b3J5GAIgASgJEiYKCXJlc291cmNlcxgDIAMoCzITLmFneC52MS5SZXNvdXJjZVJlZhIcChRyZXF1aXJlZF9lbnZpcm9ubWVudBgEIAMoCRIPCgdoYXJuZXNzGAUgASgJIuwBCg5SZXNvbHV0aW9uU3RlcBItCgZjaGFuZ2UYASABKA4yHS5hZ3gudjEuUmVzb2x1dGlvblN0ZXAuQ2hhbmdlEiYKBnNvdXJjZRgCIAEoCzIWLmFneC52MS5Tb3VyY2VMb2NhdGlvbhIXCg9zY29wZV9kaXJlY3RvcnkYAyABKAkSFAoMYnVuZGxlX2NoYWluGAQgAygJEg8KB2hhcm5lc3MYBSABKAkiQwoGQ2hhbmdlEhYKEkNIQU5HRV9VTlNQRUNJRklFRBAAEg4KCkNIQU5HRV9BREQQARIRCg1DSEFOR0VfUkVNT1ZFEAIiXQoNUmVzb3VyY2VUcmFjZRIlCghyZXNvdXJjZRgBIAEoCzITLmFneC52MS5SZXNvdXJjZVJlZhIlCgVzdGVwcxgCIAMoCzIWLmFneC52MS5SZXNvbHV0aW9uU3RlcCIRCg9WYWxpZGF0ZVJlcXVlc3QiOwoQVmFsaWRhdGVSZXNwb25zZRInCgtkaWFnbm9zdGljcxgBIAMoCzISLmFneC52MS5EaWFnbm9zdGljIjQKDlJlc29sdmVSZXF1ZXN0EhEKCWRpcmVjdG9yeRgBIAEoCRIPCgdoYXJuZXNzGAIgASgJIq8BCg9SZXNvbHZlUmVzcG9uc2USIgoIc25hcHNob3QYASABKAsyEC5hZ3gudjEuU25hcHNob3QSJQoGdHJhY2VzGAIgAygLMhUuYWd4LnYxLlJlc291cmNlVHJhY2USJwoLZGlhZ25vc3RpY3MYAyADKAsyEi5hZ3gudjEuRGlhZ25vc3RpYxIoCghzZXR0aW5ncxgEIAMoCzIWLmFneC52MS5IYXJuZXNzU2V0dGluZyJaCg5IYXJuZXNzU2V0dGluZxIMCgRwYXRoGAEgAygJEhIKCnZhbHVlX2pzb24YAiABKAkSJgoGc291cmNlGAMgASgLMhYuYWd4LnYxLlNvdXJjZUxvY2F0aW9uIhkKF0dldENvbmZpZ3VyYXRpb25SZXF1ZXN0IqMBChhHZXRDb25maWd1cmF0aW9uUmVzcG9uc2USDAoEcm9vdBgBIAEoCRIsCgloYXJuZXNzZXMYAiADKAsyGS5hZ3gudjEuQ29uZmlndXJlZEhhcm5lc3MSGQoRc2NvcGVfZGlyZWN0b3JpZXMYAyADKAkSEAoIb3ZlcmxheXMYBCADKAkSEAoIcmV2aXNpb24YBiABKAMSDAoEaG9tZRgFIAEoCSJQChFDb25maWd1cmVkSGFybmVzcxIMCgRuYW1lGAEgASgJEg8KB2J1aWx0aW4YAiABKAkSDAoEYmFzZRgDIAEoCRIOCgZoaWRkZW4YBCABKAgiOQoYQ29tcGxldGVEaXJlY3RvcnlSZXF1ZXN0Eg4KBnByZWZpeBgBIAEoCRINCgVmaWxlcxgCIAEoCCI/ChlDb21wbGV0ZURpcmVjdG9yeVJlc3BvbnNlEhMKC2RpcmVjdG9yaWVzGAEgAygJEg0KBWZpbGVzGAIgAygJIlwKDkluc3RhbGxSZXF1ZXN0Eg4KBmZyb3plbhgBIAEoCBIPCgd1cGRhdGVzGAIgAygJEhIKCnVwZGF0ZV9hbGwYAyABKAgSFQoNcGFja2FnZXNfb25seRgEIAEoCCKXAQoPSW5zdGFsbFJlc3BvbnNlEisKCHByb2dyZXNzGAEgASgLMhcuYWd4LnYxLlBhY2thZ2VQcm9ncmVzc0gAEicKBnJlc3VsdBgCIAEoCzIVLmFneC52MS5QYWNrYWdlUmVzdWx0SAASJQoHZmFpbHVyZRgDIAEoCzISLmFneC52MS5EaWFnbm9zdGljSABCBwoFc3RhdGUiRwoPUGFja2FnZVByb2dyZXNzEhIKCnBhY2thZ2VfaWQYASABKAkSEQoJY29tcGxldGVkGAIgASgNEg0KBXRvdGFsGAMgASgNIkkKDVBhY2thZ2VSZXN1bHQSEAoId2FybmluZ3MYAyADKAkSEwoLcGFja2FnZV9pZHMYASADKAkSEQoJbG9ja2ZpbGVzGAIgAygJIogCChRQcmVwYXJlTGF1bmNoUmVxdWVzdBITCgtzbmFwc2hvdF9pZBgBIAEoCRIRCglkaXJlY3RvcnkYAiABKAkSDwoHaGFybmVzcxgDIAEoCRIYChBuYXRpdmVfYXJndW1lbnRzGAQgAygJEk8KEmVudmlyb25tZW50X2lucHV0cxgFIAMoCzIzLmFneC52MS5QcmVwYXJlTGF1bmNoUmVxdWVzdC5FbnZpcm9ubWVudElucHV0c0VudHJ5EhIKCmV4ZWN1dGFibGUYBiABKAkaOAoWRW52aXJvbm1lbnRJbnB1dHNFbnRyeRILCgNrZXkYASABKAkSDQoFdmFsdWUYAiABKAk6AjgBIj0KF1Jlc29sdmVSZXNvdXJjZXNSZXF1ZXN0EhEKCWRpcmVjdG9yeRgBIAEoCRIPCgdoYXJuZXNzGAIgASgJIrgBChhSZXNvbHZlUmVzb3VyY2VzUmVzcG9uc2USIgoIc25hcHNob3QYASABKAsyEC5hZ3gudjEuU25hcHNob3QSJQoGdHJhY2VzGAIgAygLMhUuYWd4LnYxLlJlc291cmNlVHJhY2USJwoLZGlhZ25vc3RpY3MYAyADKAsyEi5hZ3gudjEuRGlhZ25vc3RpYxIoCghzZXR0aW5ncxgEIAMoCzIWLmFneC52MS5IYXJuZXNzU2V0dGluZyL3AQoXUHJlcGFyZVJlc291cmNlc1JlcXVlc3QSEwoLc25hcHNob3RfaWQYASABKAkSEQoJZGlyZWN0b3J5GAIgASgJEg8KB2hhcm5lc3MYAyABKAkSUgoSZW52aXJvbm1lbnRfaW5wdXRzGAQgAygLMjYuYWd4LnYxLlByZXBhcmVSZXNvdXJjZXNSZXF1ZXN0LkVudmlyb25tZW50SW5wdXRzRW50cnkSFQoNbWNwX2RpcmVjdG9yeRgFIAEoCRo4ChZFbnZpcm9ubWVudElucHV0c0VudHJ5EgsKA2tleRgBIAEoCRINCgV2YWx1ZRgCIAEoCToCOAEi4gEKGFByZXBhcmVSZXNvdXJjZXNSZXNwb25zZRIeCgZsYXVuY2gYASABKAsyDi5hZ3gudjEuTGF1bmNoEigKB2NvbnRlbnQYAiABKAsyFy5hZ3gudjEuUmVzb3VyY2VDb250ZW50EiAKA21jcBgDIAMoCzITLmFneC52MS5NY3BFbmRwb2ludBINCgVncmFudBgEIAEoCRIXCg9saWZlY3ljbGVfdG9rZW4YBSABKAkSGwoTcmVuZXdfYWZ0ZXJfc2Vjb25kcxgGIAEoDRIVCg1tY3BfZGlyZWN0b3J5GAcgASgJIk0KD1Jlc291cmNlQ29udGVudBIkCgZza2lsbHMYASADKAsyFC5hZ3gudjEuU2tpbGxDb250ZW50EhQKDGluc3RydWN0aW9ucxgCIAEoCSJyCgxTa2lsbENvbnRlbnQSCgoCaWQYASABKAkSDAoEbmFtZRgCIAEoCRIOCgZkaWdlc3QYAyABKAkSIwoFZmlsZXMYBCADKAsyFC5hZ3gudjEuUmVzb3VyY2VGaWxlEhMKC2RpcmVjdG9yaWVzGAUgAygJIkEKDFJlc291cmNlRmlsZRIMCgRwYXRoGAEgASgJEg8KB2NvbnRlbnQYAiABKAwSEgoKZXhlY3V0YWJsZRgDIAEoCCImCgtNY3BFbmRwb2ludBIKCgJpZBgBIAEoCRILCgN1cmwYAiABKAkioQEKFVByZXBhcmVMYXVuY2hSZXNwb25zZRIeCgZsYXVuY2gYASABKAsyDi5hZ3gudjEuTGF1bmNoEiAKBXNwYXduGAIgASgLMhEuYWd4LnYxLlNwYXduUGxhbhIXCg9saWZlY3ljbGVfdG9rZW4YAyABKAkSGwoTcmVuZXdfYWZ0ZXJfc2Vjb25kcxgEIAEoDRIQCgh3YXJuaW5ncxgFIAMoCSL3AQoJU3Bhd25QbGFuEhEKCWFyZ3VtZW50cxgBIAMoCRIRCglkaXJlY3RvcnkYAiABKAkSSgoVZW52aXJvbm1lbnRfb3ZlcnJpZGVzGAMgAygLMisuYWd4LnYxLlNwYXduUGxhbi5FbnZpcm9ubWVudE92ZXJyaWRlc0VudHJ5EhkKEXVuc2V0X2Vudmlyb25tZW50GAQgAygJEiAKGGluaXRpYWxpemVfc3lzdGVtX3Byb21wdBgFIAEoCRo7ChlFbnZpcm9ubWVudE92ZXJyaWRlc0VudHJ5EgsKA2tleRgBIAEoCRINCgV2YWx1ZRgCIAEoCToCOAEixgMKBkxhdW5jaBIKCgJpZBgBIAEoCRITCgtzbmFwc2hvdF9pZBgCIAEoCRIRCglkaXJlY3RvcnkYAyABKAkSDwoHaGFybmVzcxgEIAEoCRIjCgVzdGF0ZRgFIAEoDjIULmFneC52MS5MYXVuY2guU3RhdGUSLgoKZXhwaXJlc19hdBgGIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXASDAoEbW9kZRgHIAEoCRIYChBwYXJlbnRfbGF1bmNoX2lkGAggASgJEhIKCnNlc3Npb25faWQYCSABKAkSIQoEa2luZBgKIAEoDjITLmFneC52MS5MYXVuY2guS2luZCJCCgRLaW5kEhQKEEtJTkRfVU5TUEVDSUZJRUQQABIQCgxLSU5EX1BST0NFU1MQARISCg5LSU5EX1JFU09VUkNFUxACIn8KBVN0YXRlEhUKEVNUQVRFX1VOU1BFQ0lGSUVEEAASEgoOU1RBVEVfUFJFUEFSRUQQARIRCg1TVEFURV9SVU5OSU5HEAISEgoOU1RBVEVfRklOSVNIRUQQAxIRCg1TVEFURV9SRVZPS0VEEAQSEQoNU1RBVEVfRVhQSVJFRBAFIloKFFJlc29sdmVUaHJlYWRSZXF1ZXN0EhgKEHBhcmVudF9sYXVuY2hfaWQYASABKAkSEQoJZGlyZWN0b3J5GAIgASgJEhUKDXNuYXBzaG90X29ubHkYAyABKAgiVQoVUmVzb2x2ZVRocmVhZFJlc3BvbnNlEiIKCHNuYXBzaG90GAEgASgLMhAuYWd4LnYxLlNuYXBzaG90EhgKEGRpc2FibGVkX3BsdWdpbnMYAiADKAki7wEKGlByZXBhcmVUaHJlYWRMYXVuY2hSZXF1ZXN0EhgKEHBhcmVudF9sYXVuY2hfaWQYASABKAkSEwoLc25hcHNob3RfaWQYAiABKAkSEQoJZGlyZWN0b3J5GAMgASgJElUKEmVudmlyb25tZW50X2lucHV0cxgEIAMoCzI5LmFneC52MS5QcmVwYXJlVGhyZWFkTGF1bmNoUmVxdWVzdC5FbnZpcm9ubWVudElucHV0c0VudHJ5GjgKFkVudmlyb25tZW50SW5wdXRzRW50cnkSCwoDa2V5GAEgASgJEg0KBXZhbHVlGAIgASgJOgI4ASKCAQobUHJlcGFyZVRocmVhZExhdW5jaFJlc3BvbnNlEh4KBmxhdW5jaBgBIAEoCzIOLmFneC52MS5MYXVuY2gSIgoGdGhyZWFkGAIgASgLMhIuYWd4LnYxLlRocmVhZFBsYW4SEAoId2FybmluZ3MYAyADKAkSDQoFZ3JhbnQYBCABKAkiUQoKVGhyZWFkUGxhbhITCgtjb25maWdfanNvbhgBIAEoCRIUCgxpbnN0cnVjdGlvbnMYAiABKAkSGAoQZGlzYWJsZWRfcGx1Z2lucxgDIAMoCSJlChJSZW5ld0xhdW5jaFJlcXVlc3QSEQoJbGF1bmNoX2lkGAEgASgJEhMKC2hhcm5lc3NfcGlkGAIgASgEEhIKCnNlc3Npb25faWQYAyABKAkSEwoLd3JhcHBlcl9waWQYBCABKAQiRQoTUmVuZXdMYXVuY2hSZXNwb25zZRIuCgpleHBpcmVzX2F0GAEgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcCLVAgoTQXR0YWNoTGF1bmNoUmVxdWVzdBIeCgZsYXVuY2gYASABKAsyDi5hZ3gudjEuTGF1bmNoEg0KBWdyYW50GAIgASgJEhcKD2xpZmVjeWNsZV90b2tlbhgDIAEoCRJOChJlbnZpcm9ubWVudF9pbnB1dHMYBCADKAsyMi5hZ3gudjEuQXR0YWNoTGF1bmNoUmVxdWVzdC5FbnZpcm9ubWVudElucHV0c0VudHJ5EhgKEG5hdGl2ZV9hcmd1bWVudHMYBSADKAkSEgoKZXhlY3V0YWJsZRgGIAEoCRInCgd0aHJlYWRzGAcgAygLMhYuYWd4LnYxLkF0dGFjaGVkVGhyZWFkEhUKDW1jcF9kaXJlY3RvcnkYCCABKAkaOAoWRW52aXJvbm1lbnRJbnB1dHNFbnRyeRILCgNrZXkYASABKAkSDQoFdmFsdWUYAiABKAk6AjgBIsQBCg5BdHRhY2hlZFRocmVhZBIeCgZsYXVuY2gYASABKAsyDi5hZ3gudjEuTGF1bmNoEg0KBWdyYW50GAIgASgJEkkKEmVudmlyb25tZW50X2lucHV0cxgDIAMoCzItLmFneC52MS5BdHRhY2hlZFRocmVhZC5FbnZpcm9ubWVudElucHV0c0VudHJ5GjgKFkVudmlyb25tZW50SW5wdXRzRW50cnkSCwoDa2V5GAEgASgJEg0KBXZhbHVlGAIgASgJOgI4ASJGChRBdHRhY2hMYXVuY2hSZXNwb25zZRIuCgpleHBpcmVzX2F0GAEgASgLMhouZ29vZ2xlLnByb3RvYnVmLlRpbWVzdGFtcCKvAQoTRmluaXNoTGF1bmNoUmVxdWVzdBIRCglsYXVuY2hfaWQYASABKAkSEwoJZXhpdF9jb2RlGAIgASgFSAASFQoLc2lnbmFsX25hbWUYAyABKAlIABIXCg1zcGF3bl9mYWlsdXJlGAQgASgJSAASFwoNdGhyZWFkX2Nsb3NlZBgFIAEoCEgAEhwKEnJlc291cmNlc19yZWxlYXNlZBgGIAEoCEgAQgkKB291dGNvbWUiFgoURmluaXNoTGF1bmNoUmVzcG9uc2UiKAoTUmV2b2tlTGF1bmNoUmVxdWVzdBIRCglsYXVuY2hfaWQYASABKAkiFgoUUmV2b2tlTGF1bmNoUmVzcG9uc2Ui8AEKCk1jcFJ1bnRpbWUSEQoJbGF1bmNoX2lkGAEgASgJEg4KBm1jcF9pZBgCIAEoCRInCgVzdGF0ZRgDIAEoDjIYLmFneC52MS5NY3BSdW50aW1lLlN0YXRlEicKC2RpYWdub3N0aWNzGAQgAygLMhIuYWd4LnYxLkRpYWdub3N0aWMibQoFU3RhdGUSFQoRU1RBVEVfVU5TUEVDSUZJRUQQABIUChBTVEFURV9DT05ORUNUSU5HEAESDwoLU1RBVEVfUkVBRFkQAhIUChBTVEFURV9ORUVEU19BVVRIEAMSEAoMU1RBVEVfRkFJTEVEEAQirgEKC1J1bnRpbWVWaWV3EhoKEmRhZW1vbl9pbnN0YW5jZV9pZBgBIAEoCRIPCgd2ZXJzaW9uGAIgASgJEiAKCGxhdW5jaGVzGAMgAygLMg4uYWd4LnYxLkxhdW5jaBInCgttY3Bfc2VydmVycxgEIAMoCzISLmFneC52MS5NY3BSdW50aW1lEicKC2RpYWdub3N0aWNzGAUgAygLMhIuYWd4LnYxLkRpYWdub3N0aWMiJwoRR2V0UnVudGltZVJlcXVlc3QSEgoKcHJvYmVfb25seRgBIAEoCCKRAQoTTGlzdFNlc3Npb25zUmVxdWVzdBI3CgZzdG9yZXMYASADKAsyJy5hZ3gudjEuTGlzdFNlc3Npb25zUmVxdWVzdC5TdG9yZXNFbnRyeRISCgpmcmVzaF9vbmx5GAIgASgIGi0KC1N0b3Jlc0VudHJ5EgsKA2tleRgBIAEoCRINCgV2YWx1ZRgCIAEoCToCOAEi4gEKDVN0b3JlZFNlc3Npb24SCgoCaWQYASABKAkSEQoJZGlyZWN0b3J5GAIgASgJEg0KBXRpdGxlGAMgASgJEg8KB21lc3NhZ2UYBCABKAkSLgoKc3RhcnRlZF9hdBgFIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXASLgoKdXBkYXRlZF9hdBgGIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXASDwoHYnVpbHRpbhgHIAEoCRIPCgdoYXJuZXNzGAggASgJEhAKCHJlY29yZGVkGAkgASgIIuoBChRMaXN0U2Vzc2lvbnNSZXNwb25zZRInCghzZXNzaW9ucxgBIAMoCzIVLmFneC52MS5TdG9yZWRTZXNzaW9uEg4KBnBhcnNlZBgCIAEoDRIOCgZjYWNoZWQYAyABKA0SEQoJbWFsZm9ybWVkGAQgASgNEjgKBmVycm9ycxgFIAMoCzIoLmFneC52MS5MaXN0U2Vzc2lvbnNSZXNwb25zZS5FcnJvcnNFbnRyeRINCgVmcmVzaBgGIAEoCBotCgtFcnJvcnNFbnRyeRILCgNrZXkYASABKAkSDQoFdmFsdWUYAiABKAk6AjgBIjoKEkdldFJ1bnRpbWVSZXNwb25zZRIkCgdydW50aW1lGAEgASgLMhMuYWd4LnYxLlJ1bnRpbWVWaWV3IhUKE1dhdGNoUnVudGltZVJlcXVlc3QiPAoUV2F0Y2hSdW50aW1lUmVzcG9uc2USJAoHcnVudGltZRgBIAEoCzITLmFneC52MS5SdW50aW1lVmlldyLEAQoMTWNwQ2FsbFNjb3BlEhEKCWxhdW5jaF9pZBgBIAEoCRIRCglkaXJlY3RvcnkYAiABKAkSCwoDYWxsGAMgASgIEkcKEmVudmlyb25tZW50X2lucHV0cxgEIAMoCzIrLmFneC52MS5NY3BDYWxsU2NvcGUuRW52aXJvbm1lbnRJbnB1dHNFbnRyeRo4ChZFbnZpcm9ubWVudElucHV0c0VudHJ5EgsKA2tleRgBIAEoCRINCgV2YWx1ZRgCIAEoCToCOAEiNQoXTGlzdE1jcFByb2Nlc3Nlc1JlcXVlc3QSCwoDY3dkGAEgASgJEg0KBXVuZGVyGAIgASgJIkEKGExpc3RNY3BQcm9jZXNzZXNSZXNwb25zZRIlCglwcm9jZXNzZXMYASADKAsyEi5hZ3gudjEuTWNwUHJvY2VzcyLdAQoKTWNwUHJvY2VzcxIOCgZtY3BfaWQYASABKAkSCwoDcGlkGAIgASgDEhEKCWRpcmVjdG9yeRgDIAEoCRINCgVzaGFyZRgEIAEoCRIMCgRyb290GAUgASgJEhIKCmxhdW5jaF9pZHMYBiADKAkSEAoIc2Vzc2lvbnMYByABKAUSLgoKc3RhcnRlZF9hdBgIIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXASLAoIc3RvcHNfYXQYCSABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wIiQKFVN0b3BNY3BQcm9jZXNzUmVxdWVzdBILCgNwaWQYASABKAMiGAoWU3RvcE1jcFByb2Nlc3NSZXNwb25zZSJXCgtTdG9wUmVxdWVzdBINCgVmb3JjZRgBIAEoCBIoCgVncmFjZRgCIAEoCzIZLmdvb2dsZS5wcm90b2J1Zi5EdXJhdGlvbhIPCgdyZXN0YXJ0GAMgASgIIiUKDFN0b3BSZXNwb25zZRIVCg1rZXB0X2xhdW5jaGVzGAEgASgNIkcKEExpc3RUb29sc1JlcXVlc3QSIwoFc2NvcGUYASABKAsyFC5hZ3gudjEuTWNwQ2FsbFNjb3BlEg4KBm1jcF9pZBgCIAEoCSI2ChFMaXN0VG9vbHNSZXNwb25zZRIhCgdzZXJ2ZXJzGAEgAygLMhAuYWd4LnYxLk1jcFRvb2xzIqwCCghNY3BUb29scxIOCgZtY3BfaWQYASABKAkSEwoLZGVzY3JpcHRpb24YAiABKAkSIwoEbG9hZBgDIAEoDjIVLmFneC52MS5NY3BUb29scy5Mb2FkEicKBXN0YXRlGAQgASgOMhguYWd4LnYxLk1jcFJ1bnRpbWUuU3RhdGUSDwoHbWVzc2FnZRgFIAEoCRITCgtzZXJ2ZXJfbmFtZRgGIAEoCRIWCg5zZXJ2ZXJfdmVyc2lvbhgHIAEoCRIeCgV0b29scxgIIAMoCzIPLmFneC52MS5NY3BUb29sEgwKBGljb24YCSABKAkiQQoETG9hZBIUChBMT0FEX1VOU1BFQ0lGSUVEEAASDwoLTE9BRF9OQVRJVkUQARISCg5MT0FEX09OX0RFTUFORBACInIKB01jcFRvb2wSDAoEbmFtZRgBIAEoCRINCgV0aXRsZRgCIAEoCRITCgtkZXNjcmlwdGlvbhgDIAEoCRIZChFpbnB1dF9zY2hlbWFfanNvbhgEIAEoCRIaChJvdXRwdXRfc2NoZW1hX2pzb24YBSABKAkibAoPQ2FsbFRvb2xSZXF1ZXN0EiMKBXNjb3BlGAEgASgLMhQuYWd4LnYxLk1jcENhbGxTY29wZRIOCgZtY3BfaWQYAiABKAkSDAoEdG9vbBgDIAEoCRIWCg5hcmd1bWVudHNfanNvbhgEIAEoCSJbChBDYWxsVG9vbFJlc3BvbnNlEhAKCGlzX2Vycm9yGAEgASgIEhQKDGNvbnRlbnRfanNvbhgCIAEoCRIfChdzdHJ1Y3R1cmVkX2NvbnRlbnRfanNvbhgDIAEoCSIhCgxMb2dpblJlcXVlc3QSEQoJc2VjcmV0X2lkGAEgASgJImwKDUxvZ2luUmVzcG9uc2USJwoHc3RhcnRlZBgBIAEoCzIULmFneC52MS5Mb2dpblN0YXJ0ZWRIABIpCghmaW5pc2hlZBgCIAEoCzIVLmFneC52MS5Mb2dpbkZpbmlzaGVkSABCBwoFZXZlbnQiaQoMTG9naW5TdGFydGVkEhkKEWF1dGhvcml6YXRpb25fdXJsGAEgASgJEi4KCmV4cGlyZXNfYXQYAiABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEg4KBm1jcF9pZBgDIAEoCSK0AQoNTG9naW5GaW5pc2hlZBIuCgdvdXRjb21lGAEgASgOMh0uYWd4LnYxLkxvZ2luRmluaXNoZWQuT3V0Y29tZRIPCgdtZXNzYWdlGAIgASgJImIKB091dGNvbWUSFwoTT1VUQ09NRV9VTlNQRUNJRklFRBAAEhUKEU9VVENPTUVfU1VDQ0VFREVEEAESEgoOT1VUQ09NRV9GQUlMRUQQAhITCg9PVVRDT01FX0VYUElSRUQQAyI0ChBTZXRTZWNyZXRSZXF1ZXN0EhEKCXNlY3JldF9pZBgBIAEoCRINCgV2YWx1ZRgCIAEoDCITChFTZXRTZWNyZXRSZXNwb25zZSIrChZHZXRTZWNyZXRTdGF0dXNSZXF1ZXN0EhEKCXNlY3JldF9pZBgBIAEoCSJBChdHZXRTZWNyZXRTdGF0dXNSZXNwb25zZRImCghzdGF0dXNlcxgBIAMoCzIULmFneC52MS5TZWNyZXRTdGF0dXMi9gIKDFNlY3JldFN0YXR1cxIRCglzZWNyZXRfaWQYASABKAkSKQoFc3RhdGUYAiABKA4yGi5hZ3gudjEuU2VjcmV0U3RhdHVzLlN0YXRlEi4KCmV4cGlyZXNfYXQYAyABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEicKC2RpYWdub3N0aWNzGAQgAygLMhIuYWd4LnYxLkRpYWdub3N0aWMSDAoEa2luZBgFIAEoCRIOCgZtY3BfaWQYBiABKAkSEQoJb25fZGVtYW5kGAcgASgIEhMKC3JlZnJlc2hhYmxlGAggASgIIogBCgVTdGF0ZRIVChFTVEFURV9VTlNQRUNJRklFRBAAEhYKElNUQVRFX1VOQ09ORklHVVJFRBABEhEKDVNUQVRFX1BFTkRJTkcQAhIPCgtTVEFURV9SRUFEWRADEhUKEVNUQVRFX05FRURTX0xPR0lOEAQSFQoRU1RBVEVfVU5BVkFJTEFCTEUQBSIvCg1Mb2dvdXRSZXF1ZXN0EhEKCXNlY3JldF9pZBgBIAEoCRILCgNhbGwYAiABKAgiEAoOTG9nb3V0UmVzcG9uc2UiKAoTUm90YXRlU2VjcmV0UmVxdWVzdBIRCglzZWNyZXRfaWQYASABKAkiRgoUUm90YXRlU2VjcmV0UmVzcG9uc2USLgoKZXhwaXJlc19hdBgBIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXAi2QEKCkRlZmluaXRpb24SDAoEa2luZBgBIAEoCRIKCgJpZBgCIAEoCRImCgZzb3VyY2UYAyABKAsyFi5hZ3gudjEuU291cmNlTG9jYXRpb24SJQoFdmFsdWUYBCABKAsyFi5nb29nbGUucHJvdG9idWYuVmFsdWUSDwoHcGFja2FnZRgFIAEoCRIRCglyZWFkX29ubHkYBiABKAgSEwoLZGVzY3JpcHRpb24YByABKAkSDAoEcGF0aBgIIAEoCRINCgVjb2xvchgJIAEoCRIMCgRpY29uGAogASgJIhgKFkxpc3REZWZpbml0aW9uc1JlcXVlc3QiQgoXTGlzdERlZmluaXRpb25zUmVzcG9uc2USJwoLZGVmaW5pdGlvbnMYASADKAsyEi5hZ3gudjEuRGVmaW5pdGlvbiIjCg9HZXRTa2lsbFJlcXVlc3QSEAoIc2tpbGxfaWQYASABKAkiWwoQR2V0U2tpbGxSZXNwb25zZRImCgpkZWZpbml0aW9uGAEgASgLMhIuYWd4LnYxLkRlZmluaXRpb24SEAoIc2tpbGxfbWQYAiABKAkSDQoFZmlsZXMYAyADKAkiHwoPUmVhZEZpbGVSZXF1ZXN0EgwKBHBhdGgYASABKAkiVAoQUmVhZEZpbGVSZXNwb25zZRIMCgRwYXRoGAEgASgJEg8KB2NvbnRlbnQYAiABKAkSDgoGYmluYXJ5GAMgASgIEhEKCXJlYWRfb25seRgEIAEoCCLxAQoMQ29uZmlnQ2hhbmdlEiYKBmRlZmluZRgBIAEoCzIULmFneC52MS5EZWZpbmVDaGFuZ2VIABIqCgh1bmRlZmluZRgCIAEoCzIWLmFneC52MS5VbmRlZmluZUNoYW5nZUgAEikKBmVuYWJsZRgDIAEoCzIXLmFneC52MS5TZWxlY3Rpb25DaGFuZ2VIABIqCgdkaXNhYmxlGAQgASgLMhcuYWd4LnYxLlNlbGVjdGlvbkNoYW5nZUgAEiwKC2FkZF9wYWNrYWdlGAYgASgLMhUuYWd4LnYxLlBhY2thZ2VTb3VyY2VIAEIICgZjaGFuZ2UiTwoMRGVmaW5lQ2hhbmdlEgwKBGtpbmQYASABKAkSCgoCaWQYAiABKAkSJQoFdmFsdWUYAyABKAsyFi5nb29nbGUucHJvdG9idWYuVmFsdWUiKgoOVW5kZWZpbmVDaGFuZ2USDAoEa2luZBgBIAEoCRIKCgJpZBgCIAEoCSJRCg9TZWxlY3Rpb25DaGFuZ2USDAoEa2luZBgBIAEoCRIKCgJpZBgCIAEoCRIRCglkaXJlY3RvcnkYAyABKAkSEQoJaGFybmVzc2VzGAQgAygJIjkKGFdyaXRlUmVzb3VyY2VGaWxlUmVxdWVzdBIMCgRwYXRoGAEgASgJEg8KB2NvbnRlbnQYAiABKAkiGwoZV3JpdGVSZXNvdXJjZUZpbGVSZXNwb25zZSJFCg1QYWNrYWdlU291cmNlEgoKAmlkGAEgASgJEgsKA2dpdBgCIAEoCRILCgNyZWYYAyABKAkSDgoGc3ViZGlyGAQgASgJIl4KGlVwZGF0ZUNvbmZpZ3VyYXRpb25SZXF1ZXN0EiUKB2NoYW5nZXMYASADKAsyFC5hZ3gudjEuQ29uZmlnQ2hhbmdlEhkKEWV4cGVjdGVkX3JldmlzaW9uGAIgASgDIi8KG1VwZGF0ZUNvbmZpZ3VyYXRpb25SZXNwb25zZRIQCghyZXZpc2lvbhgBIAEoAyIkChVMaXN0U291cmNlUmVmc1JlcXVlc3QSCwoDZ2l0GAEgASgJIjkKFkxpc3RTb3VyY2VSZWZzUmVzcG9uc2USHwoEcmVmcxgBIAMoCzIRLmFneC52MS5Tb3VyY2VSZWYiRwoJU291cmNlUmVmEgwKBG5hbWUYASABKAkSCwoDdGFnGAIgASgIEg4KBmNvbW1pdBgDIAEoCRIPCgdkZWZhdWx0GAQgASgIIkAKFEluc3BlY3RTb3VyY2VSZXF1ZXN0EgsKA2dpdBgBIAEoCRILCgNyZWYYAiABKAkSDgoGc3ViZGlyGAMgASgJIkwKFUluc3BlY3RTb3VyY2VSZXNwb25zZRIQCghyZXZpc2lvbhgBIAEoCRIMCgRoYXNoGAIgASgJEhMKC2RpcmVjdG9yaWVzGAMgAygJIiYKFFByZXZpZXdTb3VyY2VSZXF1ZXN0Eg4KBnNvdXJjZRgBIAEoCSJ5ChVQcmV2aWV3U291cmNlUmVzcG9uc2USEgoKcHJldmlld19pZBgBIAEoCRImCgdwYWNrYWdlGAIgASgLMhUuYWd4LnYxLlBhY2thZ2VTb3VyY2USJAoGc2tpbGxzGAMgAygLMhQuYWd4LnYxLlByZXZpZXdTa2lsbCJSCgxQcmV2aWV3U2tpbGwSDAoEbmFtZRgBIAEoCRITCgtkZXNjcmlwdGlvbhgCIAEoCRIQCghza2lsbF9tZBgDIAEoCRINCgVmaWxlcxgEIAMoCSJJChZSZWFkUHJldmlld0ZpbGVSZXF1ZXN0EhIKCnByZXZpZXdfaWQYASABKAkSDQoFc2tpbGwYAiABKAkSDAoEcGF0aBgDIAEoCSI6ChdSZWFkUHJldmlld0ZpbGVSZXNwb25zZRIPCgdjb250ZW50GAEgASgJEg4KBmJpbmFyeRgCIAEoCCIeChxHZXRMb2NhbENvbmZpZ3VyYXRpb25SZXF1ZXN0Iu4BCh1HZXRMb2NhbENvbmZpZ3VyYXRpb25SZXNwb25zZRIpCghkb2N1bWVudBgBIAEoCzIXLmdvb2dsZS5wcm90b2J1Zi5TdHJ1Y3QSEAoIcmV2aXNpb24YAiABKAMSRQoIcGFja2FnZXMYAyADKAsyMy5hZ3gudjEuR2V0TG9jYWxDb25maWd1cmF0aW9uUmVzcG9uc2UuUGFja2FnZXNFbnRyeRpJCg1QYWNrYWdlc0VudHJ5EgsKA2tleRgBIAEoCRInCgV2YWx1ZRgCIAEoCzIYLmFneC52MS5Mb2NhbFBhY2thZ2VMb2NrOgI4ASKVAQoQTG9jYWxQYWNrYWdlTG9jaxIMCgRwYXRoGAEgASgJEgsKA2dpdBgCIAEoCRILCgNyZWYYAyABKAkSEwoLc291cmNlX2hhc2gYBCABKAkSDgoGc3ViZGlyGAUgASgJEhAKCHJldmlzaW9uGAYgASgJEgwKBGhhc2gYByABKAkSFAoMZGVwZW5kZW5jaWVzGAggAygJItYBChlSZXNvbHZlUGFzc3Rocm91Z2hSZXF1ZXN0Eg8KB2hhcm5lc3MYASABKAkSGAoQbmF0aXZlX2FyZ3VtZW50cxgCIAMoCRJUChJlbnZpcm9ubWVudF9pbnB1dHMYAyADKAsyOC5hZ3gudjEuUmVzb2x2ZVBhc3N0aHJvdWdoUmVxdWVzdC5FbnZpcm9ubWVudElucHV0c0VudHJ5GjgKFkVudmlyb25tZW50SW5wdXRzRW50cnkSCwoDa2V5GAEgASgJEg0KBXZhbHVlGAIgASgJOgI4ASKtAQoaUmVzb2x2ZVBhc3N0aHJvdWdoUmVzcG9uc2USEQoJYXJndW1lbnRzGAEgAygJEkgKC2Vudmlyb25tZW50GAIgAygLMjMuYWd4LnYxLlJlc29sdmVQYXNzdGhyb3VnaFJlc3BvbnNlLkVudmlyb25tZW50RW50cnkaMgoQRW52aXJvbm1lbnRFbnRyeRILCgNrZXkYASABKAkSDQoFdmFsdWUYAiABKAk6AjgBIuoBChNSZWNvcmRMYXVuY2hSZXF1ZXN0EgoKAmlkGAEgASgJEg8KB2hhcm5lc3MYAiABKAkSDwoHYnVpbHRpbhgDIAEoCRINCgVzdG9yZRgEIAEoCRIRCglkaXJlY3RvcnkYBSABKAkSLgoKc3RhcnRlZF9hdBgGIAEoCzIaLmdvb2dsZS5wcm90b2J1Zi5UaW1lc3RhbXASLwoLZmluaXNoZWRfYXQYByABKAsyGi5nb29nbGUucHJvdG9idWYuVGltZXN0YW1wEhMKC3Nlc3Npb25faWRzGAggAygJEg0KBWV4YWN0GAkgASgIIhYKFFJlY29yZExhdW5jaFJlc3BvbnNlMpEHChRDb25maWd1cmF0aW9uU2VydmljZRJkChVHZXRMb2NhbENvbmZpZ3VyYXRpb24SJC5hZ3gudjEuR2V0TG9jYWxDb25maWd1cmF0aW9uUmVxdWVzdBolLmFneC52MS5HZXRMb2NhbENvbmZpZ3VyYXRpb25SZXNwb25zZRJbChJSZXNvbHZlUGFzc3Rocm91Z2gSIS5hZ3gudjEuUmVzb2x2ZVBhc3N0aHJvdWdoUmVxdWVzdBoiLmFneC52MS5SZXNvbHZlUGFzc3Rocm91Z2hSZXNwb25zZRI9CghWYWxpZGF0ZRIXLmFneC52MS5WYWxpZGF0ZVJlcXVlc3QaGC5hZ3gudjEuVmFsaWRhdGVSZXNwb25zZRI6CgdSZXNvbHZlEhYuYWd4LnYxLlJlc29sdmVSZXF1ZXN0GhcuYWd4LnYxLlJlc29sdmVSZXNwb25zZRJVChBHZXRDb25maWd1cmF0aW9uEh8uYWd4LnYxLkdldENvbmZpZ3VyYXRpb25SZXF1ZXN0GiAuYWd4LnYxLkdldENvbmZpZ3VyYXRpb25SZXNwb25zZRJYChFDb21wbGV0ZURpcmVjdG9yeRIgLmFneC52MS5Db21wbGV0ZURpcmVjdG9yeVJlcXVlc3QaIS5hZ3gudjEuQ29tcGxldGVEaXJlY3RvcnlSZXNwb25zZRJSCg9MaXN0RGVmaW5pdGlvbnMSHi5hZ3gudjEuTGlzdERlZmluaXRpb25zUmVxdWVzdBofLmFneC52MS5MaXN0RGVmaW5pdGlvbnNSZXNwb25zZRI9CghHZXRTa2lsbBIXLmFneC52MS5HZXRTa2lsbFJlcXVlc3QaGC5hZ3gudjEuR2V0U2tpbGxSZXNwb25zZRI9CghSZWFkRmlsZRIXLmFneC52MS5SZWFkRmlsZVJlcXVlc3QaGC5hZ3gudjEuUmVhZEZpbGVSZXNwb25zZRJYChFXcml0ZVJlc291cmNlRmlsZRIgLmFneC52MS5Xcml0ZVJlc291cmNlRmlsZVJlcXVlc3QaIS5hZ3gudjEuV3JpdGVSZXNvdXJjZUZpbGVSZXNwb25zZRJeChNVcGRhdGVDb25maWd1cmF0aW9uEiIuYWd4LnYxLlVwZGF0ZUNvbmZpZ3VyYXRpb25SZXF1ZXN0GiMuYWd4LnYxLlVwZGF0ZUNvbmZpZ3VyYXRpb25SZXNwb25zZTKPAwoOUGFja2FnZVNlcnZpY2USPAoHSW5zdGFsbBIWLmFneC52MS5JbnN0YWxsUmVxdWVzdBoXLmFneC52MS5JbnN0YWxsUmVzcG9uc2UwARJMCg1QcmV2aWV3U291cmNlEhwuYWd4LnYxLlByZXZpZXdTb3VyY2VSZXF1ZXN0Gh0uYWd4LnYxLlByZXZpZXdTb3VyY2VSZXNwb25zZRJSCg9SZWFkUHJldmlld0ZpbGUSHi5hZ3gudjEuUmVhZFByZXZpZXdGaWxlUmVxdWVzdBofLmFneC52MS5SZWFkUHJldmlld0ZpbGVSZXNwb25zZRJPCg5MaXN0U291cmNlUmVmcxIdLmFneC52MS5MaXN0U291cmNlUmVmc1JlcXVlc3QaHi5hZ3gudjEuTGlzdFNvdXJjZVJlZnNSZXNwb25zZRJMCg1JbnNwZWN0U291cmNlEhwuYWd4LnYxLkluc3BlY3RTb3VyY2VSZXF1ZXN0Gh0uYWd4LnYxLkluc3BlY3RTb3VyY2VSZXNwb25zZTL/BAoNTGF1bmNoU2VydmljZRJJCgxSZWNvcmRMYXVuY2gSGy5hZ3gudjEuUmVjb3JkTGF1bmNoUmVxdWVzdBocLmFneC52MS5SZWNvcmRMYXVuY2hSZXNwb25zZRJMCg1QcmVwYXJlTGF1bmNoEhwuYWd4LnYxLlByZXBhcmVMYXVuY2hSZXF1ZXN0Gh0uYWd4LnYxLlByZXBhcmVMYXVuY2hSZXNwb25zZRJGCgtSZW5ld0xhdW5jaBIaLmFneC52MS5SZW5ld0xhdW5jaFJlcXVlc3QaGy5hZ3gudjEuUmVuZXdMYXVuY2hSZXNwb25zZRJJCgxGaW5pc2hMYXVuY2gSGy5hZ3gudjEuRmluaXNoTGF1bmNoUmVxdWVzdBocLmFneC52MS5GaW5pc2hMYXVuY2hSZXNwb25zZRJJCgxSZXZva2VMYXVuY2gSGy5hZ3gudjEuUmV2b2tlTGF1bmNoUmVxdWVzdBocLmFneC52MS5SZXZva2VMYXVuY2hSZXNwb25zZRJMCg1SZXNvbHZlVGhyZWFkEhwuYWd4LnYxLlJlc29sdmVUaHJlYWRSZXF1ZXN0Gh0uYWd4LnYxLlJlc29sdmVUaHJlYWRSZXNwb25zZRJeChNQcmVwYXJlVGhyZWFkTGF1bmNoEiIuYWd4LnYxLlByZXBhcmVUaHJlYWRMYXVuY2hSZXF1ZXN0GiMuYWd4LnYxLlByZXBhcmVUaHJlYWRMYXVuY2hSZXNwb25zZRJJCgxBdHRhY2hMYXVuY2gSGy5hZ3gudjEuQXR0YWNoTGF1bmNoUmVxdWVzdBocLmFneC52MS5BdHRhY2hMYXVuY2hSZXNwb25zZTK/AQoPUmVzb3VyY2VTZXJ2aWNlElUKEFJlc29sdmVSZXNvdXJjZXMSHy5hZ3gudjEuUmVzb2x2ZVJlc291cmNlc1JlcXVlc3QaIC5hZ3gudjEuUmVzb2x2ZVJlc291cmNlc1Jlc3BvbnNlElUKEFByZXBhcmVSZXNvdXJjZXMSHy5hZ3gudjEuUHJlcGFyZVJlc291cmNlc1JlcXVlc3QaIC5hZ3gudjEuUHJlcGFyZVJlc291cmNlc1Jlc3BvbnNlMssECg5SdW50aW1lU2VydmljZRJDCgpHZXRSdW50aW1lEhkuYWd4LnYxLkdldFJ1bnRpbWVSZXF1ZXN0GhouYWd4LnYxLkdldFJ1bnRpbWVSZXNwb25zZRJLCgxXYXRjaFJ1bnRpbWUSGy5hZ3gudjEuV2F0Y2hSdW50aW1lUmVxdWVzdBocLmFneC52MS5XYXRjaFJ1bnRpbWVSZXNwb25zZTABEksKDExpc3RTZXNzaW9ucxIbLmFneC52MS5MaXN0U2Vzc2lvbnNSZXF1ZXN0GhwuYWd4LnYxLkxpc3RTZXNzaW9uc1Jlc3BvbnNlMAESQAoJTGlzdFRvb2xzEhguYWd4LnYxLkxpc3RUb29sc1JlcXVlc3QaGS5hZ3gudjEuTGlzdFRvb2xzUmVzcG9uc2USPQoIQ2FsbFRvb2wSFy5hZ3gudjEuQ2FsbFRvb2xSZXF1ZXN0GhguYWd4LnYxLkNhbGxUb29sUmVzcG9uc2USVQoQTGlzdE1jcFByb2Nlc3NlcxIfLmFneC52MS5MaXN0TWNwUHJvY2Vzc2VzUmVxdWVzdBogLmFneC52MS5MaXN0TWNwUHJvY2Vzc2VzUmVzcG9uc2USTwoOU3RvcE1jcFByb2Nlc3MSHS5hZ3gudjEuU3RvcE1jcFByb2Nlc3NSZXF1ZXN0Gh4uYWd4LnYxLlN0b3BNY3BQcm9jZXNzUmVzcG9uc2USMQoEU3RvcBITLmFneC52MS5TdG9wUmVxdWVzdBoULmFneC52MS5TdG9wUmVzcG9uc2Uy4QIKDVNlY3JldFNlcnZpY2USNgoFTG9naW4SFC5hZ3gudjEuTG9naW5SZXF1ZXN0GhUuYWd4LnYxLkxvZ2luUmVzcG9uc2UwARJACglTZXRTZWNyZXQSGC5hZ3gudjEuU2V0U2VjcmV0UmVxdWVzdBoZLmFneC52MS5TZXRTZWNyZXRSZXNwb25zZRJSCg9HZXRTZWNyZXRTdGF0dXMSHi5hZ3gudjEuR2V0U2VjcmV0U3RhdHVzUmVxdWVzdBofLmFneC52MS5HZXRTZWNyZXRTdGF0dXNSZXNwb25zZRI3CgZMb2dvdXQSFS5hZ3gudjEuTG9nb3V0UmVxdWVzdBoWLmFneC52MS5Mb2dvdXRSZXNwb25zZRJJCgxSb3RhdGVTZWNyZXQSGy5hZ3gudjEuUm90YXRlU2VjcmV0UmVxdWVzdBocLmFneC52MS5Sb3RhdGVTZWNyZXRSZXNwb25zZUItWitnaXRodWIuY29tL3RhcmlrMDItb3JnL2FneC9nZW4vYWd4L3YxO2FneHYxYgZwcm90bzM", [
	file_google_protobuf_duration,
	file_google_protobuf_timestamp,
	file_google_protobuf_struct
]);
/**
* @generated from enum agx.v1.Launch.Kind
*/
let Launch_Kind = /* @__PURE__ */ function(Launch_Kind) {
	/**
	* @generated from enum value: KIND_UNSPECIFIED = 0;
	*/
	Launch_Kind[Launch_Kind["UNSPECIFIED"] = 0] = "UNSPECIFIED";
	/**
	* @generated from enum value: KIND_PROCESS = 1;
	*/
	Launch_Kind[Launch_Kind["PROCESS"] = 1] = "PROCESS";
	/**
	* @generated from enum value: KIND_RESOURCES = 2;
	*/
	Launch_Kind[Launch_Kind["RESOURCES"] = 2] = "RESOURCES";
	return Launch_Kind;
}({});
/**
* @generated from service agx.v1.LaunchService
*/
const LaunchService = /*@__PURE__*/ serviceDesc(file_agx_v1_agx, 2);
/**
* Configuration consumption does not require a process launched by agx or a shared filesystem.
*
* @generated from service agx.v1.ResourceService
*/
const ResourceService = /*@__PURE__*/ serviceDesc(file_agx_v1_agx, 3);
// ../client/dist/error-Dg-0N9zM.mjs
var ConnectionError = class extends Schema$1.TaggedError()("ConnectionError", {
	operation: Schema$1.String,
	reason: Schema$1.Literals([
		"options",
		"bootstrap",
		"discovery"
	]),
	cause: Schema$1.optionalKey(Schema$1.Union([Schema$1.instanceOf(PlatformError.PlatformError), Schema$1.instanceOf(Schema$1.SchemaError)]))
}) {
	get message() {
		return `AGX ${this.operation} failed (${this.reason})`;
	}
};
const EndState = Schema$1.NullOr(Schema$1.Literals([
	"expired",
	"revoked",
	"finished"
]));
var DaemonError = class extends Schema$1.TaggedError()("DaemonError", {
	operation: Schema$1.String,
	code: Schema$1.Enum(Code),
	ended: EndState,
	cause: Schema$1.instanceOf(ConnectError)
}) {
	get message() {
		return `AGX ${this.operation} failed (RPC code ${this.code}${this.ended === null ? "" : `, ${this.ended}`})`;
	}
};
// ../../node_modules/.pnpm/@connectrpc+connect-web@2.3.0_@bufbuild+protobuf@2.16.0_@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0_/node_modules/@connectrpc/connect-web/dist/esm/assert-fetch-api.js
/**
* Asserts that the fetch API is available.
*/
function assertFetchApi() {
	try {
		new Headers();
	} catch (_) {
		throw new Error("connect-web requires the fetch API. Are you running on an old version of Node.js?");
	}
}
// ../../node_modules/.pnpm/@connectrpc+connect-web@2.3.0_@bufbuild+protobuf@2.16.0_@connectrpc+connect@2.3.0_@bufbuild+protobuf@2.16.0_/node_modules/@connectrpc/connect-web/dist/esm/connect-transport.js
var __await = function(v) {
	return this instanceof __await ? (this.v = v, this) : new __await(v);
};
var __asyncGenerator = function(thisArg, _arguments, generator) {
	if (!Symbol.asyncIterator) throw new TypeError("Symbol.asyncIterator is not defined.");
	var g = generator.apply(thisArg, _arguments || []), i, q = [];
	return i = Object.create((typeof AsyncIterator === "function" ? AsyncIterator : Object).prototype), verb("next"), verb("throw"), verb("return", awaitReturn), i[Symbol.asyncIterator] = function() {
		return this;
	}, i;
	function awaitReturn(f) {
		return function(v) {
			return Promise.resolve(v).then(f, reject);
		};
	}
	function verb(n, f) {
		if (g[n]) {
			i[n] = function(v) {
				return new Promise(function(a, b) {
					q.push([
						n,
						v,
						a,
						b
					]) > 1 || resume(n, v);
				});
			};
			if (f) i[n] = f(i[n]);
		}
	}
	function resume(n, v) {
		try {
			step(g[n](v));
		} catch (e) {
			settle(q[0][3], e);
		}
	}
	function step(r) {
		r.value instanceof __await ? Promise.resolve(r.value.v).then(fulfill, reject) : settle(q[0][2], r);
	}
	function fulfill(value) {
		resume("next", value);
	}
	function reject(value) {
		resume("throw", value);
	}
	function settle(f, v) {
		if (f(v), q.shift(), q.length) resume(q[0][0], q[0][1]);
	}
};
const fetchOptions = { redirect: "error" };
/**
* Create a Transport for the Connect protocol, which makes unary and
* server-streaming methods available to web browsers. It uses the fetch
* API to make HTTP requests.
*/
function createConnectTransport(options) {
	var _a;
	assertFetchApi();
	const useBinaryFormat = (_a = options.useBinaryFormat) !== null && _a !== void 0 ? _a : false;
	return {
		async unary(method, signal, timeoutMs, header, message, contextValues) {
			const { serialize, parse } = createClientMethodSerializers(method, useBinaryFormat, options.jsonOptions, options.binaryOptions);
			timeoutMs = timeoutMs === void 0 ? options.defaultTimeoutMs : timeoutMs <= 0 ? void 0 : timeoutMs;
			return await runUnaryCall({
				interceptors: options.interceptors,
				signal,
				timeoutMs,
				req: {
					stream: false,
					service: method.parent,
					method,
					requestMethod: "POST",
					url: createMethodUrl(options.baseUrl, method),
					header: requestHeader(method.methodKind, useBinaryFormat, timeoutMs, header, false),
					contextValues: contextValues !== null && contextValues !== void 0 ? contextValues : createContextValues(),
					message
				},
				next: async (req) => {
					var _a;
					const useGet = options.useHttpGet === true && method.idempotency === MethodOptions_IdempotencyLevel.NO_SIDE_EFFECTS;
					let body = null;
					if (useGet) req = transformConnectPostToGetRequest(req, serialize(req.message), useBinaryFormat);
					else body = serialize(req.message);
					const response = await ((_a = options.fetch) !== null && _a !== void 0 ? _a : globalThis.fetch)(req.url, Object.assign(Object.assign({}, fetchOptions), {
						method: req.requestMethod,
						headers: req.header,
						signal: req.signal,
						body
					}));
					const { isUnaryError, unaryError } = validateResponse(method.methodKind, useBinaryFormat, response.status, response.headers);
					if (isUnaryError) throw errorFromJson(await response.json(), appendHeaders(...trailerDemux(response.headers)), unaryError);
					const [demuxedHeader, demuxedTrailer] = trailerDemux(response.headers);
					return {
						stream: false,
						service: method.parent,
						method,
						header: demuxedHeader,
						message: useBinaryFormat ? parse(new Uint8Array(await response.arrayBuffer())) : fromJson(method.output, await response.json(), getJsonOptions(options.jsonOptions)),
						trailer: demuxedTrailer
					};
				}
			});
		},
		async stream(method, signal, timeoutMs, header, input, contextValues) {
			const { serialize, parse } = createClientMethodSerializers(method, useBinaryFormat, options.jsonOptions, options.binaryOptions);
			function parseResponseBody(body, trailerTarget, header, signal) {
				return __asyncGenerator(this, arguments, function* parseResponseBody_1() {
					const reader = createEnvelopeReadableStream(body).getReader();
					let endStreamReceived = false;
					for (;;) {
						const result = yield __await(reader.read());
						if (result.done) break;
						const { flags, data } = result.value;
						if ((flags & 1) === 1) throw new ConnectError(`protocol error: received unsupported compressed output`, Code.Internal);
						if ((flags & 2) === 2) {
							endStreamReceived = true;
							const endStream = endStreamFromJson(data);
							if (endStream.error) {
								const error = endStream.error;
								header.forEach((value, key) => {
									error.metadata.append(key, value);
								});
								throw error;
							}
							endStream.metadata.forEach((value, key) => trailerTarget.set(key, value));
							continue;
						}
						yield yield __await(parse(data));
					}
					if (!endStreamReceived) throw "missing EndStreamResponse";
				});
			}
			async function createRequestBody(input) {
				if (method.methodKind != "server_streaming") throw "The fetch API does not support streaming request bodies";
				const r = await input[Symbol.asyncIterator]().next();
				if (r.done == true) throw "missing request message";
				return encodeEnvelope(0, serialize(r.value));
			}
			timeoutMs = timeoutMs === void 0 ? options.defaultTimeoutMs : timeoutMs <= 0 ? void 0 : timeoutMs;
			return await runStreamingCall({
				interceptors: options.interceptors,
				timeoutMs,
				signal,
				req: {
					stream: true,
					service: method.parent,
					method,
					requestMethod: "POST",
					url: createMethodUrl(options.baseUrl, method),
					header: requestHeader(method.methodKind, useBinaryFormat, timeoutMs, header, false),
					contextValues: contextValues !== null && contextValues !== void 0 ? contextValues : createContextValues(),
					message: input
				},
				next: async (req) => {
					var _a;
					const fRes = await ((_a = options.fetch) !== null && _a !== void 0 ? _a : globalThis.fetch)(req.url, Object.assign(Object.assign({}, fetchOptions), {
						method: req.requestMethod,
						headers: req.header,
						signal: req.signal,
						body: await createRequestBody(req.message)
					}));
					validateResponse(method.methodKind, useBinaryFormat, fRes.status, fRes.headers);
					if (fRes.body === null) throw "missing response body";
					const trailer = new Headers();
					return Object.assign(Object.assign({}, req), {
						header: fRes.headers,
						trailer,
						message: parseResponseBody(fRes.body, trailer, fRes.headers, req.signal)
					});
				}
			});
		}
	};
}
// ../client/dist/AgxClient-tmvsEEov.mjs
var AgxClient = class extends Context.Service()("agx-client/AgxClient") {};
const call = (target, operation, token, request) => Effect.tryPromise((signal) => request(createConnectTransport({ baseUrl: target.url.origin }), {
	signal,
	timeoutMs: 1e4,
	headers: { Authorization: `Bearer ${token}` }
})).pipe(Effect.catchTag("UnknownError", (error) => {
	if (!Schema$1.is(Schema$1.instanceOf(ConnectError))(error.cause)) return Effect.die(error.cause);
	const cause = error.cause;
	const ended = cause.metadata.get("Agx-Launch-Ended");
	return Effect.fail(new DaemonError({
		operation,
		code: cause.code,
		ended: Schema$1.is(EndState)(ended) ? ended : null,
		cause
	}));
}));
const makeAgxClient = Effect.fn("makeAgxClient")(() => Effect.sync(() => {
	const resolve = Effect.fn("AgxClient.resolve")((target, input) => call(target, "resolve resources", target.token, (transport, options) => createClient(ResourceService, transport).resolveResources(input, options)));
	const prepare = Effect.fn("AgxClient.prepare")((target, input) => call(target, "prepare resources", target.token, (transport, options) => createClient(ResourceService, transport).prepareResources(input, options)));
	const renew = Effect.fn("AgxClient.renew")((target, launchId, token) => call(target, "renew resources", token, (transport, options) => createClient(LaunchService, transport).renewLaunch({ launchId }, options)), Effect.asVoid);
	const attach = Effect.fn("AgxClient.attach")((target, input) => call(target, "attach resources", target.token, (transport, options) => createClient(LaunchService, transport).attachLaunch(input, options)), Effect.asVoid);
	const release = Effect.fn("AgxClient.release")((target, launchId, token) => call(target, "release resources", token, (transport, options) => createClient(LaunchService, transport).finishLaunch({
		launchId,
		outcome: {
			case: "resourcesReleased",
			value: true
		}
	}, options)), Effect.asVoid, Effect.catchTag("DaemonError", (error) => error.code === Code.NotFound || error.ended !== null ? Effect.void : Effect.fail(error)));
	return AgxClient.of({
		resolve,
		prepare,
		renew,
		attach,
		release
	});
}));
const AgxClientLive = Layer.effect(AgxClient, makeAgxClient());
// ../client/dist/schema-DSb-TiTz.mjs
const ConnectionOptions = Schema$1.Struct({
	executable: Schema$1.optionalKey(Schema$1.NonEmptyString),
	config: Schema$1.optionalKey(Schema$1.String),
	discovery: Schema$1.optionalKey(Schema$1.String)
});
const LifecycleToken = Schema$1.NonEmptyString.pipe(Schema$1.brand("agx/LifecycleToken"));
const McpGrant = Schema$1.NonEmptyString.pipe(Schema$1.brand("agx/McpGrant"));
const ManagementToken = Schema$1.NonEmptyString.pipe(Schema$1.brand("agx/ManagementToken"));
const Bootstrap = Schema$1.Struct({
	version: Schema$1.Literal(1),
	data: Schema$1.Struct({ discovery: Schema$1.String })
});
const Discovery = Schema$1.Struct({
	url: Schema$1.URLFromString.check(Schema$1.makeFilter((url) => url.protocol === "http:" && url.hostname === "127.0.0.1" && url.port !== "" && url.username === "" && url.password === "" && url.pathname === "/" && url.search === "" && url.hash === "")),
	token: ManagementToken,
	instance_id: Schema$1.NonEmptyString,
	config: Schema$1.NonEmptyString
});
// ../client/dist/AgxConnection-B3GTJPl2.mjs
var AgxConnection = class extends Context.Service()("agx-client/AgxConnection") {};
const makeAgxConnection = Effect.fn("makeAgxConnection")(function* (input = {}) {
	const options = yield* Schema$1.decodeUnknownEffect(ConnectionOptions)(input).pipe(Effect.catchTag("SchemaError", (cause) => Effect.fail(new ConnectionError({
		operation: "connection options",
		reason: "options",
		cause
	}))));
	const fs = yield* FileSystem.FileSystem;
	const spawner = yield* ChildProcessSpawner$1.ChildProcessSpawner;
	const executable = options.executable ?? "agx";
	const explicitDiscovery = options.discovery ?? null;
	let discoveryFile = explicitDiscovery;
	const bootstrap = Effect.fn("AgxConnection.bootstrap")(function* (instance = null) {
		const args = ["--json"];
		if (options.config !== void 0) args.push("--config", options.config);
		args.push("daemon", "connect");
		if (instance !== null) args.push("--recover-instance", instance);
		const output = yield* spawner.string(ChildProcess.make(executable, args, { stderr: "ignore" })).pipe(Effect.catchTag("PlatformError", (cause) => Effect.fail(new ConnectionError({
			operation: "daemon bootstrap",
			reason: "bootstrap",
			cause
		}))));
		const result = yield* Schema$1.decodeUnknownEffect(Schema$1.fromJsonString(Bootstrap))(output).pipe(Effect.catchTag("SchemaError", (cause) => Effect.fail(new ConnectionError({
			operation: "daemon bootstrap response",
			reason: "bootstrap",
			cause
		}))));
		discoveryFile = result.data.discovery;
		return result.data.discovery;
	});
	const readDiscovery = Effect.fn("AgxConnection.readDiscovery")(function* (allowStartup) {
		if (discoveryFile === null && !allowStartup) return yield* new ConnectionError({
			operation: "daemon discovery",
			reason: "discovery"
		});
		const path = discoveryFile === null ? yield* bootstrap() : discoveryFile;
		const text = yield* fs.readFileString(path).pipe(Effect.catchReason("PlatformError", "NotFound", (_reason, cause) => explicitDiscovery === null && allowStartup ? bootstrap().pipe(Effect.flatMap((next) => fs.readFileString(next))) : Effect.fail(new ConnectionError({
			operation: "daemon discovery",
			reason: "discovery",
			cause
		}))), Effect.catchTag("PlatformError", (cause) => Effect.fail(new ConnectionError({
			operation: "daemon discovery",
			reason: "discovery",
			cause
		}))));
		return yield* Schema$1.decodeUnknownEffect(Schema$1.fromJsonString(Discovery))(text).pipe(Effect.catchTag("SchemaError", (cause) => Effect.fail(new ConnectionError({
			operation: "daemon discovery document",
			reason: "discovery",
			cause
		}))));
	});
	const discover = Effect.fn("AgxConnection.discover")(() => readDiscovery(true));
	const poll = Effect.fn("AgxConnection.poll")(() => readDiscovery(false));
	const reconnect = Effect.fn("AgxConnection.reconnect")((instance) => explicitDiscovery === null ? bootstrap(instance).pipe(Effect.andThen(readDiscovery(false))) : readDiscovery(false));
	return AgxConnection.of({
		discover,
		poll,
		reconnect
	});
});
const AgxConnectionLive = (options = {}) => Layer.effect(AgxConnection, makeAgxConnection(options));
// src/schema.ts
const Options = Schema$1.Struct({
	...ConnectionOptions.fields,
	harness: Schema$1.optionalKey(Schema$1.String),
	scopeDirectory: Schema$1.optionalKey(Schema$1.String),
	mcpDirectory: Schema$1.optionalKey(Schema$1.String),
	codemode: Schema$1.optionalKey(Schema$1.Boolean)
});
const ResourcePath = Schema$1.String.check(Schema$1.makeFilter((value) => value.length > 0 && !value.includes("\\") && !value.includes(":") && !value.includes("\0") && value.split("/").every((part) => part !== "" && part !== "." && part !== "..")));
const SkillTree = Schema$1.Struct({
	id: Schema$1.NonEmptyString,
	name: Schema$1.NonEmptyString,
	digest: Schema$1.String.check(Schema$1.isPattern(/^[a-f0-9]{64}$/)),
	files: Schema$1.Array(Schema$1.Struct({
		path: ResourcePath,
		content: Schema$1.Uint8Array,
		executable: Schema$1.Boolean
	})),
	directories: Schema$1.Array(ResourcePath)
});
const Frontmatter = Schema$1.Struct({
	description: Schema$1.optionalKey(Schema$1.String),
	"disable-model-invocation": Schema$1.optionalKey(Schema$1.Boolean)
});
const Snapshot = Schema$1.Struct({
	id: Schema$1.NonEmptyString,
	directory: Schema$1.NonEmptyString,
	harness: Schema$1.String,
	requiredEnvironment: Schema$1.Array(Schema$1.NonEmptyString)
});
const LeaseEnvelope = Schema$1.Struct({
	launch: Schema$1.Struct({
		id: Schema$1.NonEmptyString,
		snapshotId: Schema$1.NonEmptyString,
		directory: Schema$1.NonEmptyString,
		harness: Schema$1.String,
		kind: Schema$1.Literal(Launch_Kind.RESOURCES)
	}),
	lifecycleToken: LifecycleToken
});
const Prepared = Schema$1.Struct({
	...LeaseEnvelope.fields,
	content: Schema$1.Struct({
		skills: Schema$1.Array(SkillTree),
		instructions: Schema$1.String
	}),
	mcp: Schema$1.Array(Schema$1.Struct({
		id: Schema$1.NonEmptyString,
		url: Schema$1.URLFromString
	})),
	grant: McpGrant,
	renewAfterSeconds: Schema$1.Int.check(Schema$1.isGreaterThan(0)),
	mcpDirectory: Schema$1.String
});
Schema.Int.check(Schema.isGreaterThan(0));
Schema.Int.check(Schema.isGreaterThanOrEqualTo(0));
Schema.String.pipe(Schema.brand("RelativePath"), Schema.annotate({ identifier: "RelativePath" }));
const AbsolutePath = Schema.String.pipe(Schema.brand("AbsolutePath"), Schema.annotate({ identifier: "AbsolutePath" }));
const optional = (schema) => Schema.optionalKey(schema).pipe(Schema.decodeTo(Schema.optional(Schema.toType(schema)), {
	decode: SchemaGetter.passthrough({ strict: false }),
	encode: SchemaGetter.transformOptional(Option.filter((value) => value !== void 0))
}));
const withStatics = (schema, methods) => Object.defineProperties(schema, Object.getOwnPropertyDescriptors(methods));
const statics = (methods) => (schema) => withStatics(schema, methods(schema));
Schema.Finite.pipe(Schema.decodeTo(Schema.DateTimeUtc, {
	decode: SchemaGetter.transform((value) => DateTime.makeUnsafe(value)),
	encode: SchemaGetter.transform((value) => DateTime.toEpochMillis(value))
}));
// ../../node_modules/.pnpm/@opencode+schema@2.2.5/node_modules/@opencode/schema/dist/identifier.js
const chars = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz";
let lastTimestamp = 0;
let counter = 0;
function ascending() {
	return create(false);
}
function create(descending, timestamp = Date.now()) {
	if (timestamp !== lastTimestamp) {
		lastTimestamp = timestamp;
		counter = 0;
	}
	counter++;
	const current = BigInt(timestamp) * 4096n + BigInt(counter);
	const value = descending ? ~current : current;
	const time = Array.from({ length: 6 }, (_, index) => Number(value >> BigInt(40 - 8 * index) & 255n).toString(16).padStart(2, "0")).join("");
	const bytes = crypto.getRandomValues(/* @__PURE__ */ new Uint8Array(14));
	return time + Array.from(bytes, (byte) => chars[byte % 62]).join("");
}
// ../../node_modules/.pnpm/@opencode+schema@2.2.5/node_modules/@opencode/schema/dist/project-id.js
const ProjectID = Schema.String.pipe(Schema.brand("Project.ID"), Schema.annotate({ identifier: "Project.ID" }), statics((schema) => ({ global: schema.make("global") })));
// ../../node_modules/.pnpm/@opencode+schema@2.2.5/node_modules/@opencode/schema/dist/workspace-id.js
const WorkspaceID = Schema.String.check(Schema.isStartingWith("wrk")).pipe(Schema.brand("Workspace.ID"), Schema.annotate({ identifier: "Workspace.ID" }), statics((schema) => {
	const create = () => schema.make("wrk_" + ascending());
	return {
		ascending: (id) => {
			if (!id) return create();
			return schema.make(id);
		},
		create
	};
}));
// ../../node_modules/.pnpm/@opencode+schema@2.2.5/node_modules/@opencode/schema/dist/location.js
const Ref = Schema.Struct({
	directory: AbsolutePath,
	workspaceID: optional(WorkspaceID)
}).annotate({ identifier: "Location.Ref" });
Schema.Struct(Struct.omit(Ref.fields, ["workspaceID"])).annotate({ identifier: "Location.PublicRef" });
var Info$1 = class extends Schema.Class("Location.Info")({
	directory: AbsolutePath,
	workspaceID: optional(WorkspaceID),
	project: Schema.Struct({
		id: ProjectID,
		directory: AbsolutePath,
		canonical: AbsolutePath
	})
}) {};
Schema.Struct(Struct.omit(Info$1.fields, ["workspaceID"])).annotate({ identifier: "Location.PublicInfo" });
// ../../node_modules/.pnpm/@opencode+schema@2.2.5/node_modules/@opencode/schema/dist/event.js
const ID$1 = Schema.String.check(Schema.isStartingWith("evt_")).pipe(Schema.brand("Event.ID"), Schema.annotate({ identifier: "Event.ID" }), statics((schema) => ({ create: () => schema.make("evt_" + ascending()) })));
/**
* Position in one aggregate's durable log. Values originate from the durable
* event envelope and synced markers;
* `after` cursors accept only values that came from those sources.
*/
const Seq = Schema.Int.check(Schema.isGreaterThanOrEqualTo(0)).pipe(Schema.brand("Event.Seq"), Schema.annotate({ identifier: "Event.Seq" }));
/** Durable schema version of one event type, from the event definition that committed it. */
const Version = Schema.Int.check(Schema.isGreaterThanOrEqualTo(1)).pipe(Schema.brand("Event.Version"), Schema.annotate({ identifier: "Event.Version" }));
Schema.Struct({
	aggregateID: Schema.String,
	seq: Seq,
	version: Version
});
function ephemeral(input) {
	const data = Schema.Struct(input.schema);
	return Schema.Struct({
		id: ID$1,
		created: Schema.Finite,
		metadata: optional(Schema.Record(Schema.String, Schema.Unknown)),
		type: Schema.Literal(input.type),
		location: optional(Ref),
		data
	}).annotate({ identifier: input.identifier ?? input.type }).pipe(statics(() => ({
		type: input.type,
		durability: "ephemeral",
		durable: void 0,
		data
	})));
}
function inventory(...definitions) {
	return Object.freeze(definitions);
}
// ../../node_modules/.pnpm/@opencode+schema@2.2.5/node_modules/@opencode/schema/dist/skill.js
const ID = Schema.String.pipe(Schema.brand("Skill.ID"), Schema.annotate({ identifier: "Skill.ID" }));
const Name = Schema.String.pipe(Schema.brand("Skill.Name"), Schema.annotate({ identifier: "Skill.Name" }));
const DirectorySource = Schema.Struct({
	type: Schema.tag("directory"),
	path: AbsolutePath
}).annotate({ identifier: "Skill.DirectorySource" });
const UrlSource = Schema.Struct({
	type: Schema.tag("url"),
	url: Schema.String
}).annotate({ identifier: "Skill.UrlSource" });
const Info = Schema.Struct({
	id: ID,
	name: Name,
	description: Schema.String.pipe(optional),
	autoinvoke: Schema.Boolean.pipe(optional),
	path: AbsolutePath,
	content: Schema.String
}).annotate({ identifier: "Skill.Info" });
inventory(ephemeral({
	type: "skill.updated",
	schema: {}
}));
const EmbeddedSource = Schema.Struct({
	type: Schema.tag("embedded"),
	skill: Schema.suspend(() => Info)
}).annotate({ identifier: "Skill.EmbeddedSource" });
Object.assign(Schema.Union([
	DirectorySource,
	UrlSource,
	EmbeddedSource
]).pipe(Schema.toTaggedUnion("type"), Schema.annotate({ identifier: "Skill.Source" })), {
	equals: (a, b) => {
		if (a.type !== b.type) return false;
		if (a.type === "directory" && b.type === "directory") return a.path === b.path;
		if (a.type === "url" && b.type === "url") return a.url === b.url;
		if (a.type === "embedded" && b.type === "embedded") return a.skill.id === b.skill.id;
		return false;
	},
	key: (source) => source.type === "directory" ? `directory:${source.path}` : source.type === "url" ? `url:${source.url}` : `embedded:${source.skill.id}`
});
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/nodes/identity.js
var require_identity = /* @__PURE__ */ __commonJSMin(((exports) => {
	const ALIAS = Symbol.for("yaml.alias");
	const DOC = Symbol.for("yaml.document");
	const MAP = Symbol.for("yaml.map");
	const PAIR = Symbol.for("yaml.pair");
	const SCALAR = Symbol.for("yaml.scalar");
	const SEQ = Symbol.for("yaml.seq");
	const NODE_TYPE = Symbol.for("yaml.node.type");
	const isAlias = (node) => !!node && typeof node === "object" && node[NODE_TYPE] === ALIAS;
	const isDocument = (node) => !!node && typeof node === "object" && node[NODE_TYPE] === DOC;
	const isMap = (node) => !!node && typeof node === "object" && node[NODE_TYPE] === MAP;
	const isPair = (node) => !!node && typeof node === "object" && node[NODE_TYPE] === PAIR;
	const isScalar = (node) => !!node && typeof node === "object" && node[NODE_TYPE] === SCALAR;
	const isSeq = (node) => !!node && typeof node === "object" && node[NODE_TYPE] === SEQ;
	function isCollection(node) {
		if (node && typeof node === "object") switch (node[NODE_TYPE]) {
			case MAP:
			case SEQ: return true;
		}
		return false;
	}
	function isNode(node) {
		if (node && typeof node === "object") switch (node[NODE_TYPE]) {
			case ALIAS:
			case MAP:
			case SCALAR:
			case SEQ: return true;
		}
		return false;
	}
	const hasAnchor = (node) => (isScalar(node) || isCollection(node)) && !!node.anchor;
	exports.ALIAS = ALIAS;
	exports.DOC = DOC;
	exports.MAP = MAP;
	exports.NODE_TYPE = NODE_TYPE;
	exports.PAIR = PAIR;
	exports.SCALAR = SCALAR;
	exports.SEQ = SEQ;
	exports.hasAnchor = hasAnchor;
	exports.isAlias = isAlias;
	exports.isCollection = isCollection;
	exports.isDocument = isDocument;
	exports.isMap = isMap;
	exports.isNode = isNode;
	exports.isPair = isPair;
	exports.isScalar = isScalar;
	exports.isSeq = isSeq;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/visit.js
var require_visit = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	const BREAK = Symbol("break visit");
	const SKIP = Symbol("skip children");
	const REMOVE = Symbol("remove node");
	/**
	* Apply a visitor to an AST node or document.
	*
	* Walks through the tree (depth-first) starting from `node`, calling a
	* `visitor` function with three arguments:
	*   - `key`: For sequence values and map `Pair`, the node's index in the
	*     collection. Within a `Pair`, `'key'` or `'value'`, correspondingly.
	*     `null` for the root node.
	*   - `node`: The current node.
	*   - `path`: The ancestry of the current node.
	*
	* The return value of the visitor may be used to control the traversal:
	*   - `undefined` (default): Do nothing and continue
	*   - `visit.SKIP`: Do not visit the children of this node, continue with next
	*     sibling
	*   - `visit.BREAK`: Terminate traversal completely
	*   - `visit.REMOVE`: Remove the current node, then continue with the next one
	*   - `Node`: Replace the current node, then continue by visiting it
	*   - `number`: While iterating the items of a sequence or map, set the index
	*     of the next step. This is useful especially if the index of the current
	*     node has changed.
	*
	* If `visitor` is a single function, it will be called with all values
	* encountered in the tree, including e.g. `null` values. Alternatively,
	* separate visitor functions may be defined for each `Map`, `Pair`, `Seq`,
	* `Alias` and `Scalar` node. To define the same visitor function for more than
	* one node type, use the `Collection` (map and seq), `Value` (map, seq & scalar)
	* and `Node` (alias, map, seq & scalar) targets. Of all these, only the most
	* specific defined one will be used for each node.
	*/
	function visit(node, visitor) {
		const visitor_ = initVisitor(visitor);
		if (identity.isDocument(node)) {
			if (visit_(null, node.contents, visitor_, Object.freeze([node])) === REMOVE) node.contents = null;
		} else visit_(null, node, visitor_, Object.freeze([]));
	}
	/** Terminate visit traversal completely */
	visit.BREAK = BREAK;
	/** Do not visit the children of the current node */
	visit.SKIP = SKIP;
	/** Remove the current node */
	visit.REMOVE = REMOVE;
	function visit_(key, node, visitor, path) {
		const ctrl = callVisitor(key, node, visitor, path);
		if (identity.isNode(ctrl) || identity.isPair(ctrl)) {
			replaceNode(key, path, ctrl);
			return visit_(key, ctrl, visitor, path);
		}
		if (typeof ctrl !== "symbol") {
			if (identity.isCollection(node)) {
				path = Object.freeze(path.concat(node));
				for (let i = 0; i < node.items.length; ++i) {
					const ci = visit_(i, node.items[i], visitor, path);
					if (typeof ci === "number") i = ci - 1;
					else if (ci === BREAK) return BREAK;
					else if (ci === REMOVE) {
						node.items.splice(i, 1);
						i -= 1;
					}
				}
			} else if (identity.isPair(node)) {
				path = Object.freeze(path.concat(node));
				const ck = visit_("key", node.key, visitor, path);
				if (ck === BREAK) return BREAK;
				else if (ck === REMOVE) node.key = null;
				const cv = visit_("value", node.value, visitor, path);
				if (cv === BREAK) return BREAK;
				else if (cv === REMOVE) node.value = null;
			}
		}
		return ctrl;
	}
	/**
	* Apply an async visitor to an AST node or document.
	*
	* Walks through the tree (depth-first) starting from `node`, calling a
	* `visitor` function with three arguments:
	*   - `key`: For sequence values and map `Pair`, the node's index in the
	*     collection. Within a `Pair`, `'key'` or `'value'`, correspondingly.
	*     `null` for the root node.
	*   - `node`: The current node.
	*   - `path`: The ancestry of the current node.
	*
	* The return value of the visitor may be used to control the traversal:
	*   - `Promise`: Must resolve to one of the following values
	*   - `undefined` (default): Do nothing and continue
	*   - `visit.SKIP`: Do not visit the children of this node, continue with next
	*     sibling
	*   - `visit.BREAK`: Terminate traversal completely
	*   - `visit.REMOVE`: Remove the current node, then continue with the next one
	*   - `Node`: Replace the current node, then continue by visiting it
	*   - `number`: While iterating the items of a sequence or map, set the index
	*     of the next step. This is useful especially if the index of the current
	*     node has changed.
	*
	* If `visitor` is a single function, it will be called with all values
	* encountered in the tree, including e.g. `null` values. Alternatively,
	* separate visitor functions may be defined for each `Map`, `Pair`, `Seq`,
	* `Alias` and `Scalar` node. To define the same visitor function for more than
	* one node type, use the `Collection` (map and seq), `Value` (map, seq & scalar)
	* and `Node` (alias, map, seq & scalar) targets. Of all these, only the most
	* specific defined one will be used for each node.
	*/
	async function visitAsync(node, visitor) {
		const visitor_ = initVisitor(visitor);
		if (identity.isDocument(node)) {
			if (await visitAsync_(null, node.contents, visitor_, Object.freeze([node])) === REMOVE) node.contents = null;
		} else await visitAsync_(null, node, visitor_, Object.freeze([]));
	}
	/** Terminate visit traversal completely */
	visitAsync.BREAK = BREAK;
	/** Do not visit the children of the current node */
	visitAsync.SKIP = SKIP;
	/** Remove the current node */
	visitAsync.REMOVE = REMOVE;
	async function visitAsync_(key, node, visitor, path) {
		const ctrl = await callVisitor(key, node, visitor, path);
		if (identity.isNode(ctrl) || identity.isPair(ctrl)) {
			replaceNode(key, path, ctrl);
			return visitAsync_(key, ctrl, visitor, path);
		}
		if (typeof ctrl !== "symbol") {
			if (identity.isCollection(node)) {
				path = Object.freeze(path.concat(node));
				for (let i = 0; i < node.items.length; ++i) {
					const ci = await visitAsync_(i, node.items[i], visitor, path);
					if (typeof ci === "number") i = ci - 1;
					else if (ci === BREAK) return BREAK;
					else if (ci === REMOVE) {
						node.items.splice(i, 1);
						i -= 1;
					}
				}
			} else if (identity.isPair(node)) {
				path = Object.freeze(path.concat(node));
				const ck = await visitAsync_("key", node.key, visitor, path);
				if (ck === BREAK) return BREAK;
				else if (ck === REMOVE) node.key = null;
				const cv = await visitAsync_("value", node.value, visitor, path);
				if (cv === BREAK) return BREAK;
				else if (cv === REMOVE) node.value = null;
			}
		}
		return ctrl;
	}
	function initVisitor(visitor) {
		if (typeof visitor === "object" && (visitor.Collection || visitor.Node || visitor.Value)) return Object.assign({
			Alias: visitor.Node,
			Map: visitor.Node,
			Scalar: visitor.Node,
			Seq: visitor.Node
		}, visitor.Value && {
			Map: visitor.Value,
			Scalar: visitor.Value,
			Seq: visitor.Value
		}, visitor.Collection && {
			Map: visitor.Collection,
			Seq: visitor.Collection
		}, visitor);
		return visitor;
	}
	function callVisitor(key, node, visitor, path) {
		if (typeof visitor === "function") return visitor(key, node, path);
		if (identity.isMap(node)) return visitor.Map?.(key, node, path);
		if (identity.isSeq(node)) return visitor.Seq?.(key, node, path);
		if (identity.isPair(node)) return visitor.Pair?.(key, node, path);
		if (identity.isScalar(node)) return visitor.Scalar?.(key, node, path);
		if (identity.isAlias(node)) return visitor.Alias?.(key, node, path);
	}
	function replaceNode(key, path, node) {
		const parent = path[path.length - 1];
		if (identity.isCollection(parent)) parent.items[key] = node;
		else if (identity.isPair(parent)) {
			if (key === "key") parent.key = node;
			else parent.value = node;
		} else if (identity.isDocument(parent)) parent.contents = node;
		else {
			const pt = identity.isAlias(parent) ? "alias" : "scalar";
			throw new Error(`Cannot replace node with ${pt} parent`);
		}
	}
	exports.visit = visit;
	exports.visitAsync = visitAsync;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/doc/directives.js
var require_directives = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var visit = require_visit();
	const escapeChars = {
		"!": "%21",
		",": "%2C",
		"[": "%5B",
		"]": "%5D",
		"{": "%7B",
		"}": "%7D"
	};
	const escapeTagName = (tn) => tn.replace(/[!,[\]{}]/g, (ch) => escapeChars[ch]);
	var Directives = class Directives {
		constructor(yaml, tags) {
			/**
			* The directives-end/doc-start marker `---`. If `null`, a marker may still be
			* included in the document's stringified representation.
			*/
			this.docStart = null;
			/** The doc-end marker `...`.  */
			this.docEnd = false;
			this.yaml = Object.assign({}, Directives.defaultYaml, yaml);
			this.tags = Object.assign({}, Directives.defaultTags, tags);
		}
		clone() {
			const copy = new Directives(this.yaml, this.tags);
			copy.docStart = this.docStart;
			return copy;
		}
		/**
		* During parsing, get a Directives instance for the current document and
		* update the stream state according to the current version's spec.
		*/
		atDocument() {
			const res = new Directives(this.yaml, this.tags);
			switch (this.yaml.version) {
				case "1.1":
					this.atNextDocument = true;
					break;
				case "1.2":
					this.atNextDocument = false;
					this.yaml = {
						explicit: Directives.defaultYaml.explicit,
						version: "1.2"
					};
					this.tags = Object.assign({}, Directives.defaultTags);
			}
			return res;
		}
		/**
		* @param onError - May be called even if the action was successful
		* @returns `true` on success
		*/
		add(line, onError) {
			if (this.atNextDocument) {
				this.yaml = {
					explicit: Directives.defaultYaml.explicit,
					version: "1.1"
				};
				this.tags = Object.assign({}, Directives.defaultTags);
				this.atNextDocument = false;
			}
			const parts = line.trim().split(/[ \t]+/);
			const name = parts.shift();
			switch (name) {
				case "%TAG": {
					if (parts.length !== 2) {
						onError(0, "%TAG directive should contain exactly two parts");
						if (parts.length < 2) return false;
					}
					const [handle, prefix] = parts;
					this.tags[handle] = prefix;
					return true;
				}
				case "%YAML": {
					this.yaml.explicit = true;
					if (parts.length !== 1) {
						onError(0, "%YAML directive should contain exactly one part");
						return false;
					}
					const [version] = parts;
					if (version === "1.1" || version === "1.2") {
						this.yaml.version = version;
						return true;
					} else {
						const isValid = /^\d+\.\d+$/.test(version);
						onError(6, `Unsupported YAML version ${version}`, isValid);
						return false;
					}
				}
				default:
					onError(0, `Unknown directive ${name}`, true);
					return false;
			}
		}
		/**
		* Resolves a tag, matching handles to those defined in %TAG directives.
		*
		* @returns Resolved tag, which may also be the non-specific tag `'!'` or a
		*   `'!local'` tag, or `null` if unresolvable.
		*/
		tagName(source, onError) {
			if (source === "!") return "!";
			if (source[0] !== "!") {
				onError(`Not a valid tag: ${source}`);
				return null;
			}
			if (source[1] === "<") {
				const verbatim = source.slice(2, -1);
				if (verbatim === "!" || verbatim === "!!") {
					onError(`Verbatim tags aren't resolved, so ${source} is invalid.`);
					return null;
				}
				if (source[source.length - 1] !== ">") onError("Verbatim tags must end with a >");
				return verbatim;
			}
			const [, handle, suffix] = source.match(/^(.*!)([^!]*)$/s);
			if (!suffix) onError(`The ${source} tag has no suffix`);
			const prefix = this.tags[handle];
			if (prefix) try {
				return prefix + decodeURIComponent(suffix);
			} catch (error) {
				onError(String(error));
				return null;
			}
			if (handle === "!") return source;
			onError(`Could not resolve tag: ${source}`);
			return null;
		}
		/**
		* Given a fully resolved tag, returns its printable string form,
		* taking into account current tag prefixes and defaults.
		*/
		tagString(tag) {
			for (const [handle, prefix] of Object.entries(this.tags)) if (tag.startsWith(prefix)) return handle + escapeTagName(tag.substring(prefix.length));
			return tag[0] === "!" ? tag : `!<${tag}>`;
		}
		toString(doc) {
			const lines = this.yaml.explicit ? [`%YAML ${this.yaml.version || "1.2"}`] : [];
			const tagEntries = Object.entries(this.tags);
			let tagNames;
			if (doc && tagEntries.length > 0 && identity.isNode(doc.contents)) {
				const tags = {};
				visit.visit(doc.contents, (_key, node) => {
					if (identity.isNode(node) && node.tag) tags[node.tag] = true;
				});
				tagNames = Object.keys(tags);
			} else tagNames = [];
			for (const [handle, prefix] of tagEntries) {
				if (handle === "!!" && prefix === "tag:yaml.org,2002:") continue;
				if (!doc || tagNames.some((tn) => tn.startsWith(prefix))) lines.push(`%TAG ${handle} ${prefix}`);
			}
			return lines.join("\n");
		}
	};
	Directives.defaultYaml = {
		explicit: false,
		version: "1.2"
	};
	Directives.defaultTags = { "!!": "tag:yaml.org,2002:" };
	exports.Directives = Directives;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/doc/anchors.js
var require_anchors = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var visit = require_visit();
	/**
	* Verify that the input string is a valid anchor.
	*
	* Will throw on errors.
	*/
	function anchorIsValid(anchor) {
		if (/[\x00-\x19\s,[\]{}]/.test(anchor)) {
			const msg = `Anchor must not contain whitespace or control characters: ${JSON.stringify(anchor)}`;
			throw new Error(msg);
		}
		return true;
	}
	function anchorNames(root) {
		const anchors = /* @__PURE__ */ new Set();
		visit.visit(root, { Value(_key, node) {
			if (node.anchor) anchors.add(node.anchor);
		} });
		return anchors;
	}
	/** Find a new anchor name with the given `prefix` and a one-indexed suffix. */
	function findNewAnchor(prefix, exclude) {
		for (let i = 1;; ++i) {
			const name = `${prefix}${i}`;
			if (!exclude.has(name)) return name;
		}
	}
	function createNodeAnchors(doc, prefix) {
		const aliasObjects = [];
		const sourceObjects = /* @__PURE__ */ new Map();
		let prevAnchors = null;
		return {
			onAnchor: (source) => {
				aliasObjects.push(source);
				prevAnchors ?? (prevAnchors = anchorNames(doc));
				const anchor = findNewAnchor(prefix, prevAnchors);
				prevAnchors.add(anchor);
				return anchor;
			},
			/**
			* With circular references, the source node is only resolved after all
			* of its child nodes are. This is why anchors are set only after all of
			* the nodes have been created.
			*/
			setAnchors: () => {
				for (const source of aliasObjects) {
					const ref = sourceObjects.get(source);
					if (typeof ref === "object" && ref.anchor && (identity.isScalar(ref.node) || identity.isCollection(ref.node))) ref.node.anchor = ref.anchor;
					else {
						const error = /* @__PURE__ */ new Error("Failed to resolve repeated object (this should not happen)");
						error.source = source;
						throw error;
					}
				}
			},
			sourceObjects
		};
	}
	exports.anchorIsValid = anchorIsValid;
	exports.anchorNames = anchorNames;
	exports.createNodeAnchors = createNodeAnchors;
	exports.findNewAnchor = findNewAnchor;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/doc/applyReviver.js
var require_applyReviver = /* @__PURE__ */ __commonJSMin(((exports) => {
	/**
	* Applies the JSON.parse reviver algorithm as defined in the ECMA-262 spec,
	* in section 24.5.1.1 "Runtime Semantics: InternalizeJSONProperty" of the
	* 2021 edition: https://tc39.es/ecma262/#sec-json.parse
	*
	* Includes extensions for handling Map and Set objects.
	*/
	function applyReviver(reviver, obj, key, val) {
		if (val && typeof val === "object") {
			if (Array.isArray(val)) for (let i = 0, len = val.length; i < len; ++i) {
				const v0 = val[i];
				const v1 = applyReviver(reviver, val, String(i), v0);
				if (v1 === void 0) delete val[i];
				else if (v1 !== v0) val[i] = v1;
			}
			else if (val instanceof Map) for (const k of Array.from(val.keys())) {
				const v0 = val.get(k);
				const v1 = applyReviver(reviver, val, k, v0);
				if (v1 === void 0) val.delete(k);
				else if (v1 !== v0) val.set(k, v1);
			}
			else if (val instanceof Set) for (const v0 of Array.from(val)) {
				const v1 = applyReviver(reviver, val, v0, v0);
				if (v1 === void 0) val.delete(v0);
				else if (v1 !== v0) {
					val.delete(v0);
					val.add(v1);
				}
			}
			else for (const [k, v0] of Object.entries(val)) {
				const v1 = applyReviver(reviver, val, k, v0);
				if (v1 === void 0) delete val[k];
				else if (v1 !== v0) val[k] = v1;
			}
		}
		return reviver.call(obj, key, val);
	}
	exports.applyReviver = applyReviver;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/nodes/toJS.js
var require_toJS = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	/**
	* Recursively convert any node or its contents to native JavaScript
	*
	* @param value - The input value
	* @param arg - If `value` defines a `toJSON()` method, use this
	*   as its first argument
	* @param ctx - Conversion context, originally set in Document#toJS(). If
	*   `{ keep: true }` is not set, output should be suitable for JSON
	*   stringification.
	*/
	function toJS(value, arg, ctx) {
		if (Array.isArray(value)) return value.map((v, i) => toJS(v, String(i), ctx));
		if (value && typeof value.toJSON === "function") {
			if (!ctx || !identity.hasAnchor(value)) return value.toJSON(arg, ctx);
			const data = {
				aliasCount: 0,
				count: 1,
				res: void 0
			};
			ctx.anchors.set(value, data);
			ctx.onCreate = (res) => {
				data.res = res;
				delete ctx.onCreate;
			};
			const res = value.toJSON(arg, ctx);
			if (ctx.onCreate) ctx.onCreate(res);
			return res;
		}
		if (typeof value === "bigint" && !ctx?.keep) return Number(value);
		return value;
	}
	exports.toJS = toJS;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/nodes/Node.js
var require_Node = /* @__PURE__ */ __commonJSMin(((exports) => {
	var applyReviver = require_applyReviver();
	var identity = require_identity();
	var toJS = require_toJS();
	var NodeBase = class {
		constructor(type) {
			Object.defineProperty(this, identity.NODE_TYPE, { value: type });
		}
		/** Create a copy of this node.  */
		clone() {
			const copy = Object.create(Object.getPrototypeOf(this), Object.getOwnPropertyDescriptors(this));
			if (this.range) copy.range = this.range.slice();
			return copy;
		}
		/** A plain JavaScript representation of this node. */
		toJS(doc, { mapAsMap, maxAliasCount, onAnchor, reviver } = {}) {
			if (!identity.isDocument(doc)) throw new TypeError("A document argument is required");
			const ctx = {
				anchors: /* @__PURE__ */ new Map(),
				doc,
				keep: true,
				mapAsMap: mapAsMap === true,
				mapKeyWarned: false,
				maxAliasCount: typeof maxAliasCount === "number" ? maxAliasCount : 100
			};
			const res = toJS.toJS(this, "", ctx);
			if (typeof onAnchor === "function") for (const { count, res } of ctx.anchors.values()) onAnchor(res, count);
			return typeof reviver === "function" ? applyReviver.applyReviver(reviver, { "": res }, "", res) : res;
		}
	};
	exports.NodeBase = NodeBase;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/nodes/Alias.js
var require_Alias = /* @__PURE__ */ __commonJSMin(((exports) => {
	var anchors = require_anchors();
	var visit = require_visit();
	var identity = require_identity();
	var Node = require_Node();
	var toJS = require_toJS();
	var Alias = class extends Node.NodeBase {
		constructor(source) {
			super(identity.ALIAS);
			this.source = source;
			Object.defineProperty(this, "tag", { set() {
				throw new Error("Alias nodes cannot have tags");
			} });
		}
		/**
		* Resolve the value of this alias within `doc`, finding the last
		* instance of the `source` anchor before this node.
		*/
		resolve(doc, ctx) {
			if (ctx?.maxAliasCount === 0) throw new ReferenceError("Alias resolution is disabled");
			let nodes;
			if (ctx?.aliasResolveCache) nodes = ctx.aliasResolveCache;
			else {
				nodes = [];
				visit.visit(doc, { Node: (_key, node) => {
					if (identity.isAlias(node) || identity.hasAnchor(node)) nodes.push(node);
				} });
				if (ctx) ctx.aliasResolveCache = nodes;
			}
			let found = void 0;
			for (const node of nodes) {
				if (node === this) break;
				if (node.anchor === this.source) found = node;
			}
			if (found && ctx) {
				const { anchors, doc, maxAliasCount } = ctx;
				let data = anchors.get(found);
				if (!data) {
					toJS.toJS(found, null, ctx);
					data = anchors.get(found);
				}
				/* istanbul ignore if */
				if (data?.res === void 0) throw new ReferenceError("This should not happen: Alias anchor was not resolved?");
				if (maxAliasCount >= 0) {
					data.count += 1;
					if (data.aliasCount === 0) data.aliasCount = getAliasCount(doc, found, anchors);
					if (data.count * data.aliasCount > maxAliasCount) throw new ReferenceError("Excessive alias count indicates a resource exhaustion attack");
				}
			}
			return found;
		}
		toJSON(_arg, ctx) {
			if (!ctx) return { source: this.source };
			const source = this.resolve(ctx.doc, ctx);
			if (!source) {
				const msg = `Unresolved alias (the anchor must be set before the alias): ${this.source}`;
				throw new ReferenceError(msg);
			}
			return ctx.anchors.get(source).res;
		}
		toString(ctx, _onComment, _onChompKeep) {
			const src = `*${this.source}`;
			if (ctx) {
				anchors.anchorIsValid(this.source);
				if (ctx.options.verifyAliasOrder && !ctx.anchors.has(this.source)) {
					const msg = `Unresolved alias (the anchor must be set before the alias): ${this.source}`;
					throw new Error(msg);
				}
				if (ctx.implicitKey) return `${src} `;
			}
			return src;
		}
	};
	function getAliasCount(doc, node, anchors) {
		if (identity.isAlias(node)) {
			const source = node.resolve(doc);
			const anchor = anchors && source && anchors.get(source);
			return anchor ? anchor.count * anchor.aliasCount : 0;
		} else if (identity.isCollection(node)) {
			let count = 0;
			for (const item of node.items) {
				const c = getAliasCount(doc, item, anchors);
				if (c > count) count = c;
			}
			return count;
		} else if (identity.isPair(node)) {
			const kc = getAliasCount(doc, node.key, anchors);
			const vc = getAliasCount(doc, node.value, anchors);
			return Math.max(kc, vc);
		}
		return 1;
	}
	exports.Alias = Alias;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/nodes/Scalar.js
var require_Scalar = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var Node = require_Node();
	var toJS = require_toJS();
	const isScalarValue = (value) => !value || typeof value !== "function" && typeof value !== "object";
	var Scalar = class extends Node.NodeBase {
		constructor(value) {
			super(identity.SCALAR);
			this.value = value;
		}
		toJSON(arg, ctx) {
			return ctx?.keep ? this.value : toJS.toJS(this.value, arg, ctx);
		}
		toString() {
			return String(this.value);
		}
	};
	Scalar.BLOCK_FOLDED = "BLOCK_FOLDED";
	Scalar.BLOCK_LITERAL = "BLOCK_LITERAL";
	Scalar.PLAIN = "PLAIN";
	Scalar.QUOTE_DOUBLE = "QUOTE_DOUBLE";
	Scalar.QUOTE_SINGLE = "QUOTE_SINGLE";
	exports.Scalar = Scalar;
	exports.isScalarValue = isScalarValue;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/doc/createNode.js
var require_createNode = /* @__PURE__ */ __commonJSMin(((exports) => {
	var Alias = require_Alias();
	var identity = require_identity();
	var Scalar = require_Scalar();
	const defaultTagPrefix = "tag:yaml.org,2002:";
	function findTagObject(value, tagName, tags) {
		if (tagName) {
			const match = tags.filter((t) => t.tag === tagName);
			const tagObj = match.find((t) => !t.format) ?? match[0];
			if (!tagObj) throw new Error(`Tag ${tagName} not found`);
			return tagObj;
		}
		return tags.find((t) => t.identify?.(value) && !t.format);
	}
	function createNode(value, tagName, ctx) {
		if (identity.isDocument(value)) value = value.contents;
		if (identity.isNode(value)) return value;
		if (identity.isPair(value)) {
			const map = ctx.schema[identity.MAP].createNode?.(ctx.schema, null, ctx);
			map.items.push(value);
			return map;
		}
		if (value instanceof String || value instanceof Number || value instanceof Boolean || typeof BigInt !== "undefined" && value instanceof BigInt) value = value.valueOf();
		const { aliasDuplicateObjects, onAnchor, onTagObj, schema, sourceObjects } = ctx;
		let ref = void 0;
		if (aliasDuplicateObjects && value && typeof value === "object") {
			ref = sourceObjects.get(value);
			if (ref) {
				ref.anchor ?? (ref.anchor = onAnchor(value));
				return new Alias.Alias(ref.anchor);
			} else {
				ref = {
					anchor: null,
					node: null
				};
				sourceObjects.set(value, ref);
			}
		}
		if (tagName?.startsWith("!!")) tagName = defaultTagPrefix + tagName.slice(2);
		let tagObj = findTagObject(value, tagName, schema.tags);
		if (!tagObj) {
			if (value && typeof value.toJSON === "function") value = value.toJSON();
			if (!value || typeof value !== "object") {
				const node = new Scalar.Scalar(value);
				if (ref) ref.node = node;
				return node;
			}
			tagObj = value instanceof Map ? schema[identity.MAP] : Symbol.iterator in Object(value) ? schema[identity.SEQ] : schema[identity.MAP];
		}
		if (onTagObj) {
			onTagObj(tagObj);
			delete ctx.onTagObj;
		}
		const node = tagObj?.createNode ? tagObj.createNode(ctx.schema, value, ctx) : typeof tagObj?.nodeClass?.from === "function" ? tagObj.nodeClass.from(ctx.schema, value, ctx) : new Scalar.Scalar(value);
		if (tagName) node.tag = tagName;
		else if (!tagObj.default) node.tag = tagObj.tag;
		if (ref) ref.node = node;
		return node;
	}
	exports.createNode = createNode;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/nodes/Collection.js
var require_Collection = /* @__PURE__ */ __commonJSMin(((exports) => {
	var createNode = require_createNode();
	var identity = require_identity();
	var Node = require_Node();
	function collectionFromPath(schema, path, value) {
		let v = value;
		for (let i = path.length - 1; i >= 0; --i) {
			const k = path[i];
			if (typeof k === "number" && Number.isInteger(k) && k >= 0) {
				const a = [];
				a[k] = v;
				v = a;
			} else v = /* @__PURE__ */ new Map([[k, v]]);
		}
		return createNode.createNode(v, void 0, {
			aliasDuplicateObjects: false,
			keepUndefined: false,
			onAnchor: () => {
				throw new Error("This should not happen, please report a bug.");
			},
			schema,
			sourceObjects: /* @__PURE__ */ new Map()
		});
	}
	const isEmptyPath = (path) => path == null || typeof path === "object" && !!path[Symbol.iterator]().next().done;
	var Collection = class extends Node.NodeBase {
		constructor(type, schema) {
			super(type);
			Object.defineProperty(this, "schema", {
				value: schema,
				configurable: true,
				enumerable: false,
				writable: true
			});
		}
		/**
		* Create a copy of this collection.
		*
		* @param schema - If defined, overwrites the original's schema
		*/
		clone(schema) {
			const copy = Object.create(Object.getPrototypeOf(this), Object.getOwnPropertyDescriptors(this));
			if (schema) copy.schema = schema;
			copy.items = copy.items.map((it) => identity.isNode(it) || identity.isPair(it) ? it.clone(schema) : it);
			if (this.range) copy.range = this.range.slice();
			return copy;
		}
		/**
		* Adds a value to the collection. For `!!map` and `!!omap` the value must
		* be a Pair instance or a `{ key, value }` object, which may not have a key
		* that already exists in the map.
		*/
		addIn(path, value) {
			if (isEmptyPath(path)) this.add(value);
			else {
				const [key, ...rest] = path;
				const node = this.get(key, true);
				if (identity.isCollection(node)) node.addIn(rest, value);
				else if (node === void 0 && this.schema) this.set(key, collectionFromPath(this.schema, rest, value));
				else throw new Error(`Expected YAML collection at ${key}. Remaining path: ${rest}`);
			}
		}
		/**
		* Removes a value from the collection.
		* @returns `true` if the item was found and removed.
		*/
		deleteIn(path) {
			const [key, ...rest] = path;
			if (rest.length === 0) return this.delete(key);
			const node = this.get(key, true);
			if (identity.isCollection(node)) return node.deleteIn(rest);
			else throw new Error(`Expected YAML collection at ${key}. Remaining path: ${rest}`);
		}
		/**
		* Returns item at `key`, or `undefined` if not found. By default unwraps
		* scalar values from their surrounding node; to disable set `keepScalar` to
		* `true` (collections are always returned intact).
		*/
		getIn(path, keepScalar) {
			const [key, ...rest] = path;
			const node = this.get(key, true);
			if (rest.length === 0) return !keepScalar && identity.isScalar(node) ? node.value : node;
			else return identity.isCollection(node) ? node.getIn(rest, keepScalar) : void 0;
		}
		hasAllNullValues(allowScalar) {
			return this.items.every((node) => {
				if (!identity.isPair(node)) return false;
				const n = node.value;
				return n == null || allowScalar && identity.isScalar(n) && n.value == null && !n.commentBefore && !n.comment && !n.tag;
			});
		}
		/**
		* Checks if the collection includes a value with the key `key`.
		*/
		hasIn(path) {
			const [key, ...rest] = path;
			if (rest.length === 0) return this.has(key);
			const node = this.get(key, true);
			return identity.isCollection(node) ? node.hasIn(rest) : false;
		}
		/**
		* Sets a value in this collection. For `!!set`, `value` needs to be a
		* boolean to add/remove the item from the set.
		*/
		setIn(path, value) {
			const [key, ...rest] = path;
			if (rest.length === 0) this.set(key, value);
			else {
				const node = this.get(key, true);
				if (identity.isCollection(node)) node.setIn(rest, value);
				else if (node === void 0 && this.schema) this.set(key, collectionFromPath(this.schema, rest, value));
				else throw new Error(`Expected YAML collection at ${key}. Remaining path: ${rest}`);
			}
		}
	};
	exports.Collection = Collection;
	exports.collectionFromPath = collectionFromPath;
	exports.isEmptyPath = isEmptyPath;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/stringify/stringifyComment.js
var require_stringifyComment = /* @__PURE__ */ __commonJSMin(((exports) => {
	/**
	* Stringifies a comment.
	*
	* Empty comment lines are left empty,
	* lines consisting of a single space are replaced by `#`,
	* and all other lines are prefixed with a `#`.
	*/
	const stringifyComment = (str) => str.replace(/^(?!$)(?: $)?/gm, "#");
	function indentComment(comment, indent) {
		if (/^\n+$/.test(comment)) return comment.substring(1);
		return indent ? comment.replace(/^(?! *$)/gm, indent) : comment;
	}
	const lineComment = (str, indent, comment) => str.endsWith("\n") ? indentComment(comment, indent) : comment.includes("\n") ? "\n" + indentComment(comment, indent) : (str.endsWith(" ") ? "" : " ") + comment;
	exports.indentComment = indentComment;
	exports.lineComment = lineComment;
	exports.stringifyComment = stringifyComment;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/stringify/foldFlowLines.js
var require_foldFlowLines = /* @__PURE__ */ __commonJSMin(((exports) => {
	const FOLD_FLOW = "flow";
	const FOLD_BLOCK = "block";
	const FOLD_QUOTED = "quoted";
	/**
	* Tries to keep input at up to `lineWidth` characters, splitting only on spaces
	* not followed by newlines or spaces unless `mode` is `'quoted'`. Lines are
	* terminated with `\n` and started with `indent`.
	*/
	function foldFlowLines(text, indent, mode = "flow", { indentAtStart, lineWidth = 80, minContentWidth = 20, onFold, onOverflow } = {}) {
		if (!lineWidth || lineWidth < 0) return text;
		if (lineWidth < minContentWidth) minContentWidth = 0;
		const endStep = Math.max(1 + minContentWidth, 1 + lineWidth - indent.length);
		if (text.length <= endStep) return text;
		const folds = [];
		const escapedFolds = {};
		let end = lineWidth - indent.length;
		if (typeof indentAtStart === "number") {
			if (indentAtStart > lineWidth - Math.max(2, minContentWidth)) folds.push(0);
			else end = lineWidth - indentAtStart;
		}
		let split = void 0;
		let prev = void 0;
		let overflow = false;
		let i = -1;
		let escStart = -1;
		let escEnd = -1;
		if (mode === FOLD_BLOCK) {
			i = consumeMoreIndentedLines(text, i, indent.length);
			if (i !== -1) end = i + endStep;
		}
		for (let ch; ch = text[i += 1];) {
			if (mode === FOLD_QUOTED && ch === "\\") {
				escStart = i;
				switch (text[i + 1]) {
					case "x":
						i += 3;
						break;
					case "u":
						i += 5;
						break;
					case "U":
						i += 9;
						break;
					default: i += 1;
				}
				escEnd = i;
			}
			if (ch === "\n") {
				if (mode === FOLD_BLOCK) i = consumeMoreIndentedLines(text, i, indent.length);
				end = i + indent.length + endStep;
				split = void 0;
			} else {
				if (ch === " " && prev && prev !== " " && prev !== "\n" && prev !== "	") {
					const next = text[i + 1];
					if (next && next !== " " && next !== "\n" && next !== "	") split = i;
				}
				if (i >= end) {
					if (split) {
						folds.push(split);
						end = split + endStep;
						split = void 0;
					} else if (mode === FOLD_QUOTED) {
						while (prev === " " || prev === "	") {
							prev = ch;
							ch = text[i += 1];
							overflow = true;
						}
						const j = i > escEnd + 1 ? i - 2 : escStart - 1;
						if (escapedFolds[j]) return text;
						folds.push(j);
						escapedFolds[j] = true;
						end = j + endStep;
						split = void 0;
					} else overflow = true;
				}
			}
			prev = ch;
		}
		if (overflow && onOverflow) onOverflow();
		if (folds.length === 0) return text;
		if (onFold) onFold();
		let res = text.slice(0, folds[0]);
		for (let i = 0; i < folds.length; ++i) {
			const fold = folds[i];
			const end = folds[i + 1] || text.length;
			if (fold === 0) res = `\n${indent}${text.slice(0, end)}`;
			else {
				if (mode === FOLD_QUOTED && escapedFolds[fold]) res += `${text[fold]}\\`;
				res += `\n${indent}${text.slice(fold + 1, end)}`;
			}
		}
		return res;
	}
	/**
	* Presumes `i + 1` is at the start of a line
	* @returns index of last newline in more-indented block
	*/
	function consumeMoreIndentedLines(text, i, indent) {
		let end = i;
		let start = i + 1;
		let ch = text[start];
		while (ch === " " || ch === "	") if (i < start + indent) ch = text[++i];
		else {
			do
				ch = text[++i];
			while (ch && ch !== "\n");
			end = i;
			start = i + 1;
			ch = text[start];
		}
		return end;
	}
	exports.FOLD_BLOCK = FOLD_BLOCK;
	exports.FOLD_FLOW = FOLD_FLOW;
	exports.FOLD_QUOTED = FOLD_QUOTED;
	exports.foldFlowLines = foldFlowLines;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/stringify/stringifyString.js
var require_stringifyString = /* @__PURE__ */ __commonJSMin(((exports) => {
	var Scalar = require_Scalar();
	var foldFlowLines = require_foldFlowLines();
	const getFoldOptions = (ctx, isBlock) => ({
		indentAtStart: isBlock ? ctx.indent.length : ctx.indentAtStart,
		lineWidth: ctx.options.lineWidth,
		minContentWidth: ctx.options.minContentWidth
	});
	const containsDocumentMarker = (str) => /^(%|---|\.\.\.)/m.test(str);
	function lineLengthOverLimit(str, lineWidth, indentLength) {
		if (!lineWidth || lineWidth < 0) return false;
		const limit = lineWidth - indentLength;
		const strLen = str.length;
		if (strLen <= limit) return false;
		for (let i = 0, start = 0; i < strLen; ++i) if (str[i] === "\n") {
			if (i - start > limit) return true;
			start = i + 1;
			if (strLen - start <= limit) return false;
		}
		return true;
	}
	function doubleQuotedString(value, ctx) {
		const json = JSON.stringify(value);
		if (ctx.options.doubleQuotedAsJSON) return json;
		const { implicitKey } = ctx;
		const minMultiLineLength = ctx.options.doubleQuotedMinMultiLineLength;
		const indent = ctx.indent || (containsDocumentMarker(value) ? "  " : "");
		let str = "";
		let start = 0;
		for (let i = 0, ch = json[i]; ch; ch = json[++i]) {
			if (ch === " " && json[i + 1] === "\\" && json[i + 2] === "n") {
				str += json.slice(start, i) + "\\ ";
				i += 1;
				start = i;
				ch = "\\";
			}
			if (ch === "\\") switch (json[i + 1]) {
				case "u":
					{
						str += json.slice(start, i);
						const code = json.substr(i + 2, 4);
						switch (code) {
							case "0000":
								str += "\\0";
								break;
							case "0007":
								str += "\\a";
								break;
							case "000b":
								str += "\\v";
								break;
							case "001b":
								str += "\\e";
								break;
							case "0085":
								str += "\\N";
								break;
							case "00a0":
								str += "\\_";
								break;
							case "2028":
								str += "\\L";
								break;
							case "2029":
								str += "\\P";
								break;
							default: if (code.substr(0, 2) === "00") str += "\\x" + code.substr(2);
							else str += json.substr(i, 6);
						}
						i += 5;
						start = i + 1;
					}
					break;
				case "n":
					if (implicitKey || json[i + 2] === "\"" || json.length < minMultiLineLength) i += 1;
					else {
						str += json.slice(start, i) + "\n\n";
						while (json[i + 2] === "\\" && json[i + 3] === "n" && json[i + 4] !== "\"") {
							str += "\n";
							i += 2;
						}
						str += indent;
						if (json[i + 2] === " ") str += "\\";
						i += 1;
						start = i + 1;
					}
					break;
				default: i += 1;
			}
		}
		str = start ? str + json.slice(start) : json;
		return implicitKey ? str : foldFlowLines.foldFlowLines(str, indent, foldFlowLines.FOLD_QUOTED, getFoldOptions(ctx, false));
	}
	function singleQuotedString(value, ctx) {
		if (ctx.options.singleQuote === false || ctx.implicitKey && value.includes("\n") || /[ \t]\n|\n[ \t]/.test(value)) return doubleQuotedString(value, ctx);
		const indent = ctx.indent || (containsDocumentMarker(value) ? "  " : "");
		const res = "'" + value.replace(/'/g, "''").replace(/\n+/g, `$&\n${indent}`) + "'";
		return ctx.implicitKey ? res : foldFlowLines.foldFlowLines(res, indent, foldFlowLines.FOLD_FLOW, getFoldOptions(ctx, false));
	}
	function quotedString(value, ctx) {
		const { singleQuote } = ctx.options;
		let qs;
		if (singleQuote === false) qs = doubleQuotedString;
		else {
			const hasDouble = value.includes("\"");
			const hasSingle = value.includes("'");
			if (hasDouble && !hasSingle) qs = singleQuotedString;
			else if (hasSingle && !hasDouble) qs = doubleQuotedString;
			else qs = singleQuote ? singleQuotedString : doubleQuotedString;
		}
		return qs(value, ctx);
	}
	let blockEndNewlines;
	try {
		blockEndNewlines = new RegExp("(^|(?<!\n))\n+(?!\n|$)", "g");
	} catch {
		blockEndNewlines = /\n+(?!\n|$)/g;
	}
	function blockString({ comment, type, value }, ctx, onComment, onChompKeep) {
		const { blockQuote, commentString, lineWidth } = ctx.options;
		if (!blockQuote || /\n[\t ]+$/.test(value)) return quotedString(value, ctx);
		const indent = ctx.indent || (ctx.forceBlockIndent || containsDocumentMarker(value) ? "  " : "");
		const literal = blockQuote === "literal" ? true : blockQuote === "folded" || type === Scalar.Scalar.BLOCK_FOLDED ? false : type === Scalar.Scalar.BLOCK_LITERAL ? true : !lineLengthOverLimit(value, lineWidth, indent.length);
		if (!value) return literal ? "|\n" : ">\n";
		let chomp;
		let endStart;
		for (endStart = value.length; endStart > 0; --endStart) {
			const ch = value[endStart - 1];
			if (ch !== "\n" && ch !== "	" && ch !== " ") break;
		}
		let end = value.substring(endStart);
		const endNlPos = end.indexOf("\n");
		if (endNlPos === -1) chomp = "-";
		else if (value === end || endNlPos !== end.length - 1) {
			chomp = "+";
			if (onChompKeep) onChompKeep();
		} else chomp = "";
		if (end) {
			value = value.slice(0, -end.length);
			if (end[end.length - 1] === "\n") end = end.slice(0, -1);
			end = end.replace(blockEndNewlines, `$&${indent}`);
		}
		let startWithSpace = false;
		let startEnd;
		let startNlPos = -1;
		for (startEnd = 0; startEnd < value.length; ++startEnd) {
			const ch = value[startEnd];
			if (ch === " ") startWithSpace = true;
			else if (ch === "\n") startNlPos = startEnd;
			else break;
		}
		let start = value.substring(0, startNlPos < startEnd ? startNlPos + 1 : startEnd);
		if (start) {
			value = value.substring(start.length);
			start = start.replace(/\n+/g, `$&${indent}`);
		}
		let header = (startWithSpace ? indent ? "2" : "1" : "") + chomp;
		if (comment) {
			header += " " + commentString(comment.replace(/ ?[\r\n]+/g, " "));
			if (onComment) onComment();
		}
		if (!literal) {
			const foldedValue = value.replace(/\n+/g, "\n$&").replace(/(?:^|\n)([\t ].*)(?:([\n\t ]*)\n(?![\n\t ]))?/g, "$1$2").replace(/\n+/g, `$&${indent}`);
			let literalFallback = false;
			const foldOptions = getFoldOptions(ctx, true);
			if (blockQuote !== "folded" && type !== Scalar.Scalar.BLOCK_FOLDED) foldOptions.onOverflow = () => {
				literalFallback = true;
			};
			const body = foldFlowLines.foldFlowLines(`${start}${foldedValue}${end}`, indent, foldFlowLines.FOLD_BLOCK, foldOptions);
			if (!literalFallback) return `>${header}\n${indent}${body}`;
		}
		value = value.replace(/\n+/g, `$&${indent}`);
		return `|${header}\n${indent}${start}${value}${end}`;
	}
	function plainString(item, ctx, onComment, onChompKeep) {
		const { type, value } = item;
		const { actualString, implicitKey, indent, indentStep, inFlow } = ctx;
		if (implicitKey && value.includes("\n") || inFlow && /[[\]{},]/.test(value)) return quotedString(value, ctx);
		if (/^[\n\t ,[\]{}#&*!|>'"%@`]|^[?-]$|^[?-][ \t]|[\n:][ \t]|[ \t]\n|[\n\t ]#|[\n\t :]$/.test(value)) return implicitKey || inFlow || !value.includes("\n") ? quotedString(value, ctx) : blockString(item, ctx, onComment, onChompKeep);
		if (!implicitKey && !inFlow && type !== Scalar.Scalar.PLAIN && value.includes("\n")) return blockString(item, ctx, onComment, onChompKeep);
		if (containsDocumentMarker(value)) {
			if (indent === "") {
				ctx.forceBlockIndent = true;
				return blockString(item, ctx, onComment, onChompKeep);
			} else if (implicitKey && indent === indentStep) return quotedString(value, ctx);
		}
		const str = value.replace(/\n+/g, `$&\n${indent}`);
		if (actualString) {
			const test = (tag) => tag.default && tag.tag !== "tag:yaml.org,2002:str" && tag.test?.test(str);
			const { compat, tags } = ctx.doc.schema;
			if (tags.some(test) || compat?.some(test)) return quotedString(value, ctx);
		}
		return implicitKey ? str : foldFlowLines.foldFlowLines(str, indent, foldFlowLines.FOLD_FLOW, getFoldOptions(ctx, false));
	}
	function stringifyString(item, ctx, onComment, onChompKeep) {
		const { implicitKey, inFlow } = ctx;
		const ss = typeof item.value === "string" ? item : Object.assign({}, item, { value: String(item.value) });
		let { type } = item;
		if (type !== Scalar.Scalar.QUOTE_DOUBLE) {
			if (/[\x00-\x08\x0b-\x1f\x7f-\x9f\u{D800}-\u{DFFF}]/u.test(ss.value)) type = Scalar.Scalar.QUOTE_DOUBLE;
		}
		const _stringify = (_type) => {
			switch (_type) {
				case Scalar.Scalar.BLOCK_FOLDED:
				case Scalar.Scalar.BLOCK_LITERAL: return implicitKey || inFlow ? quotedString(ss.value, ctx) : blockString(ss, ctx, onComment, onChompKeep);
				case Scalar.Scalar.QUOTE_DOUBLE: return doubleQuotedString(ss.value, ctx);
				case Scalar.Scalar.QUOTE_SINGLE: return singleQuotedString(ss.value, ctx);
				case Scalar.Scalar.PLAIN: return plainString(ss, ctx, onComment, onChompKeep);
				default: return null;
			}
		};
		let res = _stringify(type);
		if (res === null) {
			const { defaultKeyType, defaultStringType } = ctx.options;
			const t = implicitKey && defaultKeyType || defaultStringType;
			res = _stringify(t);
			if (res === null) throw new Error(`Unsupported default string type ${t}`);
		}
		return res;
	}
	exports.stringifyString = stringifyString;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/stringify/stringify.js
var require_stringify = /* @__PURE__ */ __commonJSMin(((exports) => {
	var anchors = require_anchors();
	var identity = require_identity();
	var stringifyComment = require_stringifyComment();
	var stringifyString = require_stringifyString();
	function createStringifyContext(doc, options) {
		const opt = Object.assign({
			blockQuote: true,
			commentString: stringifyComment.stringifyComment,
			defaultKeyType: null,
			defaultStringType: "PLAIN",
			directives: null,
			doubleQuotedAsJSON: false,
			doubleQuotedMinMultiLineLength: 40,
			falseStr: "false",
			flowCollectionPadding: true,
			indentSeq: true,
			lineWidth: 80,
			minContentWidth: 20,
			nullStr: "null",
			simpleKeys: false,
			singleQuote: null,
			trailingComma: false,
			trueStr: "true",
			verifyAliasOrder: true
		}, doc.schema.toStringOptions, options);
		let inFlow;
		switch (opt.collectionStyle) {
			case "block":
				inFlow = false;
				break;
			case "flow":
				inFlow = true;
				break;
			default: inFlow = null;
		}
		return {
			anchors: /* @__PURE__ */ new Set(),
			doc,
			flowCollectionPadding: opt.flowCollectionPadding ? " " : "",
			indent: "",
			indentStep: typeof opt.indent === "number" ? " ".repeat(opt.indent) : "  ",
			inFlow,
			options: opt
		};
	}
	function getTagObject(tags, item) {
		if (item.tag) {
			const match = tags.filter((t) => t.tag === item.tag);
			if (match.length > 0) return match.find((t) => t.format === item.format) ?? match[0];
		}
		let tagObj = void 0;
		let obj;
		if (identity.isScalar(item)) {
			obj = item.value;
			let match = tags.filter((t) => t.identify?.(obj));
			if (match.length > 1) {
				const testMatch = match.filter((t) => t.test);
				if (testMatch.length > 0) match = testMatch;
			}
			tagObj = match.find((t) => t.format === item.format) ?? match.find((t) => !t.format);
		} else {
			obj = item;
			tagObj = tags.find((t) => t.nodeClass && obj instanceof t.nodeClass);
		}
		if (!tagObj) {
			const name = obj?.constructor?.name ?? (obj === null ? "null" : typeof obj);
			throw new Error(`Tag not resolved for ${name} value`);
		}
		return tagObj;
	}
	function stringifyProps(node, tagObj, { anchors: anchors$1, doc }) {
		if (!doc.directives) return "";
		const props = [];
		const anchor = (identity.isScalar(node) || identity.isCollection(node)) && node.anchor;
		if (anchor && anchors.anchorIsValid(anchor)) {
			anchors$1.add(anchor);
			props.push(`&${anchor}`);
		}
		const tag = node.tag ?? (tagObj.default ? null : tagObj.tag);
		if (tag) props.push(doc.directives.tagString(tag));
		return props.join(" ");
	}
	function stringify(item, ctx, onComment, onChompKeep) {
		if (identity.isPair(item)) return item.toString(ctx, onComment, onChompKeep);
		if (identity.isAlias(item)) {
			if (ctx.doc.directives) return item.toString(ctx);
			if (ctx.resolvedAliases?.has(item)) throw new TypeError(`Cannot stringify circular structure without alias nodes`);
			else {
				if (ctx.resolvedAliases) ctx.resolvedAliases.add(item);
				else ctx.resolvedAliases = /* @__PURE__ */ new Set([item]);
				item = item.resolve(ctx.doc);
			}
		}
		let tagObj = void 0;
		const node = identity.isNode(item) ? item : ctx.doc.createNode(item, { onTagObj: (o) => tagObj = o });
		tagObj ?? (tagObj = getTagObject(ctx.doc.schema.tags, node));
		const props = stringifyProps(node, tagObj, ctx);
		if (props.length > 0) ctx.indentAtStart = (ctx.indentAtStart ?? 0) + props.length + 1;
		const str = typeof tagObj.stringify === "function" ? tagObj.stringify(node, ctx, onComment, onChompKeep) : identity.isScalar(node) ? stringifyString.stringifyString(node, ctx, onComment, onChompKeep) : node.toString(ctx, onComment, onChompKeep);
		if (!props) return str;
		return identity.isScalar(node) || str[0] === "{" || str[0] === "[" ? `${props} ${str}` : `${props}\n${ctx.indent}${str}`;
	}
	exports.createStringifyContext = createStringifyContext;
	exports.stringify = stringify;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/stringify/stringifyPair.js
var require_stringifyPair = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var Scalar = require_Scalar();
	var stringify = require_stringify();
	var stringifyComment = require_stringifyComment();
	function stringifyPair({ key, value }, ctx, onComment, onChompKeep) {
		const { allNullValues, doc, indent, indentStep, options: { commentString, indentSeq, simpleKeys } } = ctx;
		let keyComment = identity.isNode(key) && key.comment || null;
		if (simpleKeys) {
			if (keyComment) throw new Error("With simple keys, key nodes cannot have comments");
			if (identity.isCollection(key) || !identity.isNode(key) && typeof key === "object") throw new Error("With simple keys, collection cannot be used as a key value");
		}
		let explicitKey = !simpleKeys && (!key || keyComment && value == null && !ctx.inFlow || identity.isCollection(key) || (identity.isScalar(key) ? key.type === Scalar.Scalar.BLOCK_FOLDED || key.type === Scalar.Scalar.BLOCK_LITERAL : typeof key === "object"));
		ctx = Object.assign({}, ctx, {
			allNullValues: false,
			implicitKey: !explicitKey && (simpleKeys || !allNullValues),
			indent: indent + indentStep
		});
		let keyCommentDone = false;
		let chompKeep = false;
		let str = stringify.stringify(key, ctx, () => keyCommentDone = true, () => chompKeep = true);
		if (!explicitKey && !ctx.inFlow && str.length > 1024) {
			if (simpleKeys) throw new Error("With simple keys, single line scalar must not span more than 1024 characters");
			explicitKey = true;
		}
		if (ctx.inFlow) {
			if (allNullValues || value == null) {
				if (keyCommentDone && onComment) onComment();
				return str === "" ? "?" : explicitKey ? `? ${str}` : str;
			}
		} else if (allNullValues && !simpleKeys || value == null && explicitKey) {
			str = `? ${str}`;
			if (keyComment && !keyCommentDone) str += stringifyComment.lineComment(str, ctx.indent, commentString(keyComment));
			else if (chompKeep && onChompKeep) onChompKeep();
			return str;
		}
		if (keyCommentDone) keyComment = null;
		if (explicitKey) {
			if (keyComment) str += stringifyComment.lineComment(str, ctx.indent, commentString(keyComment));
			str = `? ${str}\n${indent}:`;
		} else {
			str = `${str}:`;
			if (keyComment) str += stringifyComment.lineComment(str, ctx.indent, commentString(keyComment));
		}
		let vsb, vcb, valueComment;
		if (identity.isNode(value)) {
			vsb = !!value.spaceBefore;
			vcb = value.commentBefore;
			valueComment = value.comment;
		} else {
			vsb = false;
			vcb = null;
			valueComment = null;
			if (value && typeof value === "object") value = doc.createNode(value);
		}
		ctx.implicitKey = false;
		if (!explicitKey && !keyComment && identity.isScalar(value)) ctx.indentAtStart = str.length + 1;
		chompKeep = false;
		if (!indentSeq && indentStep.length >= 2 && !ctx.inFlow && !explicitKey && identity.isSeq(value) && !value.flow && !value.tag && !value.anchor) ctx.indent = ctx.indent.substring(2);
		let valueCommentDone = false;
		const valueStr = stringify.stringify(value, ctx, () => valueCommentDone = true, () => chompKeep = true);
		let ws = " ";
		if (keyComment || vsb || vcb) {
			ws = vsb ? "\n" : "";
			if (vcb) {
				const cs = commentString(vcb);
				ws += `\n${stringifyComment.indentComment(cs, ctx.indent)}`;
			}
			if (valueStr === "" && !ctx.inFlow) {
				if (ws === "\n" && valueComment) ws = "\n\n";
			} else ws += `\n${ctx.indent}`;
		} else if (!explicitKey && identity.isCollection(value)) {
			const vs0 = valueStr[0];
			const nl0 = valueStr.indexOf("\n");
			const hasNewline = nl0 !== -1;
			const flow = ctx.inFlow ?? value.flow ?? value.items.length === 0;
			if (hasNewline || !flow) {
				let hasPropsLine = false;
				if (hasNewline && (vs0 === "&" || vs0 === "!")) {
					let sp0 = valueStr.indexOf(" ");
					if (vs0 === "&" && sp0 !== -1 && sp0 < nl0 && valueStr[sp0 + 1] === "!") sp0 = valueStr.indexOf(" ", sp0 + 1);
					if (sp0 === -1 || nl0 < sp0) hasPropsLine = true;
				}
				if (!hasPropsLine) ws = `\n${ctx.indent}`;
			}
		} else if (valueStr === "" || valueStr[0] === "\n") ws = "";
		str += ws + valueStr;
		if (ctx.inFlow) {
			if (valueCommentDone && onComment) onComment();
		} else if (valueComment && !valueCommentDone) str += stringifyComment.lineComment(str, ctx.indent, commentString(valueComment));
		else if (chompKeep && onChompKeep) onChompKeep();
		return str;
	}
	exports.stringifyPair = stringifyPair;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/log.js
var require_log = /* @__PURE__ */ __commonJSMin(((exports) => {
	var node_process = __require("process");
	function debug(logLevel, ...messages) {
		if (logLevel === "debug") console.log(...messages);
	}
	function warn(logLevel, warning) {
		if (logLevel === "debug" || logLevel === "warn") {
			if (typeof node_process.emitWarning === "function") node_process.emitWarning(warning);
			else console.warn(warning);
		}
	}
	exports.debug = debug;
	exports.warn = warn;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/yaml-1.1/merge.js
var require_merge = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var Scalar = require_Scalar();
	const MERGE_KEY = "<<";
	const merge = {
		identify: (value) => value === MERGE_KEY || typeof value === "symbol" && value.description === MERGE_KEY,
		default: "key",
		tag: "tag:yaml.org,2002:merge",
		test: /^<<$/,
		resolve: () => Object.assign(new Scalar.Scalar(Symbol(MERGE_KEY)), { addToJSMap: addMergeToJSMap }),
		stringify: () => MERGE_KEY
	};
	const isMergeKey = (ctx, key) => (merge.identify(key) || identity.isScalar(key) && (!key.type || key.type === Scalar.Scalar.PLAIN) && merge.identify(key.value)) && ctx?.doc.schema.tags.some((tag) => tag.tag === merge.tag && tag.default);
	function addMergeToJSMap(ctx, map, value) {
		const source = resolveAliasValue(ctx, value);
		if (identity.isSeq(source)) for (const it of source.items) mergeValue(ctx, map, it);
		else if (Array.isArray(source)) for (const it of source) mergeValue(ctx, map, it);
		else mergeValue(ctx, map, source);
	}
	function mergeValue(ctx, map, value) {
		const source = resolveAliasValue(ctx, value);
		if (!identity.isMap(source)) throw new Error("Merge sources must be maps or map aliases");
		const srcMap = source.toJSON(null, ctx, Map);
		for (const [key, value] of srcMap) if (map instanceof Map) {
			if (!map.has(key)) map.set(key, value);
		} else if (map instanceof Set) map.add(key);
		else if (!Object.prototype.hasOwnProperty.call(map, key)) Object.defineProperty(map, key, {
			value,
			writable: true,
			enumerable: true,
			configurable: true
		});
		return map;
	}
	function resolveAliasValue(ctx, value) {
		return ctx && identity.isAlias(value) ? value.resolve(ctx.doc, ctx) : value;
	}
	exports.addMergeToJSMap = addMergeToJSMap;
	exports.isMergeKey = isMergeKey;
	exports.merge = merge;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/nodes/addPairToJSMap.js
var require_addPairToJSMap = /* @__PURE__ */ __commonJSMin(((exports) => {
	var log = require_log();
	var merge = require_merge();
	var stringify = require_stringify();
	var identity = require_identity();
	var toJS = require_toJS();
	function addPairToJSMap(ctx, map, { key, value }) {
		if (identity.isNode(key) && key.addToJSMap) key.addToJSMap(ctx, map, value);
		else if (merge.isMergeKey(ctx, key)) merge.addMergeToJSMap(ctx, map, value);
		else {
			const jsKey = toJS.toJS(key, "", ctx);
			if (map instanceof Map) map.set(jsKey, toJS.toJS(value, jsKey, ctx));
			else if (map instanceof Set) map.add(jsKey);
			else {
				const stringKey = stringifyKey(key, jsKey, ctx);
				const jsValue = toJS.toJS(value, stringKey, ctx);
				if (stringKey in map) Object.defineProperty(map, stringKey, {
					value: jsValue,
					writable: true,
					enumerable: true,
					configurable: true
				});
				else map[stringKey] = jsValue;
			}
		}
		return map;
	}
	function stringifyKey(key, jsKey, ctx) {
		if (jsKey === null) return "";
		if (typeof jsKey !== "object") return String(jsKey);
		if (identity.isNode(key) && ctx?.doc) {
			const strCtx = stringify.createStringifyContext(ctx.doc, {});
			strCtx.anchors = /* @__PURE__ */ new Set();
			for (const node of ctx.anchors.keys()) strCtx.anchors.add(node.anchor);
			strCtx.inFlow = true;
			strCtx.inStringifyKey = true;
			const strKey = key.toString(strCtx);
			if (!ctx.mapKeyWarned) {
				let jsonStr = JSON.stringify(strKey);
				if (jsonStr.length > 40) jsonStr = jsonStr.substring(0, 36) + "...\"";
				log.warn(ctx.doc.options.logLevel, `Keys with collection values will be stringified due to JS Object restrictions: ${jsonStr}. Set mapAsMap: true to use object keys.`);
				ctx.mapKeyWarned = true;
			}
			return strKey;
		}
		return JSON.stringify(jsKey);
	}
	exports.addPairToJSMap = addPairToJSMap;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/nodes/Pair.js
var require_Pair = /* @__PURE__ */ __commonJSMin(((exports) => {
	var createNode = require_createNode();
	var stringifyPair = require_stringifyPair();
	var addPairToJSMap = require_addPairToJSMap();
	var identity = require_identity();
	function createPair(key, value, ctx) {
		return new Pair(createNode.createNode(key, void 0, ctx), createNode.createNode(value, void 0, ctx));
	}
	var Pair = class Pair {
		constructor(key, value = null) {
			Object.defineProperty(this, identity.NODE_TYPE, { value: identity.PAIR });
			this.key = key;
			this.value = value;
		}
		clone(schema) {
			let { key, value } = this;
			if (identity.isNode(key)) key = key.clone(schema);
			if (identity.isNode(value)) value = value.clone(schema);
			return new Pair(key, value);
		}
		toJSON(_, ctx) {
			const pair = ctx?.mapAsMap ? /* @__PURE__ */ new Map() : {};
			return addPairToJSMap.addPairToJSMap(ctx, pair, this);
		}
		toString(ctx, onComment, onChompKeep) {
			return ctx?.doc ? stringifyPair.stringifyPair(this, ctx, onComment, onChompKeep) : JSON.stringify(this);
		}
	};
	exports.Pair = Pair;
	exports.createPair = createPair;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/stringify/stringifyCollection.js
var require_stringifyCollection = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var stringify = require_stringify();
	var stringifyComment = require_stringifyComment();
	function stringifyCollection(collection, ctx, options) {
		return (ctx.inFlow ?? collection.flow ? stringifyFlowCollection : stringifyBlockCollection)(collection, ctx, options);
	}
	function stringifyBlockCollection({ comment, items }, ctx, { blockItemPrefix, flowChars, itemIndent, onChompKeep, onComment }) {
		const { indent, options: { commentString } } = ctx;
		const itemCtx = Object.assign({}, ctx, {
			indent: itemIndent,
			type: null
		});
		let chompKeep = false;
		const lines = [];
		for (let i = 0; i < items.length; ++i) {
			const item = items[i];
			let comment = null;
			if (identity.isNode(item)) {
				if (!chompKeep && item.spaceBefore) lines.push("");
				addCommentBefore(ctx, lines, item.commentBefore, chompKeep);
				if (item.comment) comment = item.comment;
			} else if (identity.isPair(item)) {
				const ik = identity.isNode(item.key) ? item.key : null;
				if (ik) {
					if (!chompKeep && ik.spaceBefore) lines.push("");
					addCommentBefore(ctx, lines, ik.commentBefore, chompKeep);
				}
			}
			chompKeep = false;
			let str = stringify.stringify(item, itemCtx, () => comment = null, () => chompKeep = true);
			if (comment) str += stringifyComment.lineComment(str, itemIndent, commentString(comment));
			if (chompKeep && comment) chompKeep = false;
			lines.push(blockItemPrefix + str);
		}
		let str;
		if (lines.length === 0) str = flowChars.start + flowChars.end;
		else {
			str = lines[0];
			for (let i = 1; i < lines.length; ++i) {
				const line = lines[i];
				str += line ? `\n${indent}${line}` : "\n";
			}
		}
		if (comment) {
			str += "\n" + stringifyComment.indentComment(commentString(comment), indent);
			if (onComment) onComment();
		} else if (chompKeep && onChompKeep) onChompKeep();
		return str;
	}
	function stringifyFlowCollection({ items }, ctx, { flowChars, itemIndent }) {
		const { indent, indentStep, flowCollectionPadding: fcPadding, options: { commentString } } = ctx;
		itemIndent += indentStep;
		const itemCtx = Object.assign({}, ctx, {
			indent: itemIndent,
			inFlow: true,
			type: null
		});
		let reqNewline = false;
		let linesAtValue = 0;
		const lines = [];
		for (let i = 0; i < items.length; ++i) {
			const item = items[i];
			let comment = null;
			if (identity.isNode(item)) {
				if (item.spaceBefore) lines.push("");
				addCommentBefore(ctx, lines, item.commentBefore, false);
				if (item.comment) comment = item.comment;
			} else if (identity.isPair(item)) {
				const ik = identity.isNode(item.key) ? item.key : null;
				if (ik) {
					if (ik.spaceBefore) lines.push("");
					addCommentBefore(ctx, lines, ik.commentBefore, false);
					if (ik.comment) reqNewline = true;
				}
				const iv = identity.isNode(item.value) ? item.value : null;
				if (iv) {
					if (iv.comment) comment = iv.comment;
					if (iv.commentBefore) reqNewline = true;
				} else if (item.value == null && ik?.comment) comment = ik.comment;
			}
			if (comment) reqNewline = true;
			let str = stringify.stringify(item, itemCtx, () => comment = null);
			reqNewline || (reqNewline = lines.length > linesAtValue || str.includes("\n"));
			if (i < items.length - 1) str += ",";
			else if (ctx.options.trailingComma) {
				if (ctx.options.lineWidth > 0) reqNewline || (reqNewline = lines.reduce((sum, line) => sum + line.length + 2, 2) + (str.length + 2) > ctx.options.lineWidth);
				if (reqNewline) str += ",";
			}
			if (comment) str += stringifyComment.lineComment(str, itemIndent, commentString(comment));
			lines.push(str);
			linesAtValue = lines.length;
		}
		const { start, end } = flowChars;
		if (lines.length === 0) return start + end;
		else {
			if (!reqNewline) {
				const len = lines.reduce((sum, line) => sum + line.length + 2, 2);
				reqNewline = ctx.options.lineWidth > 0 && len > ctx.options.lineWidth;
			}
			if (reqNewline) {
				let str = start;
				for (const line of lines) str += line ? `\n${indentStep}${indent}${line}` : "\n";
				return `${str}\n${indent}${end}`;
			} else return `${start}${fcPadding}${lines.join(" ")}${fcPadding}${end}`;
		}
	}
	function addCommentBefore({ indent, options: { commentString } }, lines, comment, chompKeep) {
		if (comment && chompKeep) comment = comment.replace(/^\n+/, "");
		if (comment) {
			const ic = stringifyComment.indentComment(commentString(comment), indent);
			lines.push(ic.trimStart());
		}
	}
	exports.stringifyCollection = stringifyCollection;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/nodes/YAMLMap.js
var require_YAMLMap = /* @__PURE__ */ __commonJSMin(((exports) => {
	var stringifyCollection = require_stringifyCollection();
	var addPairToJSMap = require_addPairToJSMap();
	var Collection = require_Collection();
	var identity = require_identity();
	var Pair = require_Pair();
	var Scalar = require_Scalar();
	function findPair(items, key) {
		const k = identity.isScalar(key) ? key.value : key;
		for (const it of items) if (identity.isPair(it)) {
			if (it.key === key || it.key === k) return it;
			if (identity.isScalar(it.key) && it.key.value === k) return it;
		}
	}
	var YAMLMap = class extends Collection.Collection {
		static get tagName() {
			return "tag:yaml.org,2002:map";
		}
		constructor(schema) {
			super(identity.MAP, schema);
			this.items = [];
		}
		/**
		* A generic collection parsing method that can be extended
		* to other node classes that inherit from YAMLMap
		*/
		static from(schema, obj, ctx) {
			const { keepUndefined, replacer } = ctx;
			const map = new this(schema);
			const add = (key, value) => {
				if (typeof replacer === "function") value = replacer.call(obj, key, value);
				else if (Array.isArray(replacer) && !replacer.includes(key)) return;
				if (value !== void 0 || keepUndefined) map.items.push(Pair.createPair(key, value, ctx));
			};
			if (obj instanceof Map) for (const [key, value] of obj) add(key, value);
			else if (obj && typeof obj === "object") for (const key of Object.keys(obj)) add(key, obj[key]);
			if (typeof schema.sortMapEntries === "function") map.items.sort(schema.sortMapEntries);
			return map;
		}
		/**
		* Adds a value to the collection.
		*
		* @param overwrite - If not set `true`, using a key that is already in the
		*   collection will throw. Otherwise, overwrites the previous value.
		*/
		add(pair, overwrite) {
			let _pair;
			if (identity.isPair(pair)) _pair = pair;
			else if (!pair || typeof pair !== "object" || !("key" in pair)) _pair = new Pair.Pair(pair, pair?.value);
			else _pair = new Pair.Pair(pair.key, pair.value);
			const prev = findPair(this.items, _pair.key);
			const sortEntries = this.schema?.sortMapEntries;
			if (prev) {
				if (!overwrite) throw new Error(`Key ${_pair.key} already set`);
				if (identity.isScalar(prev.value) && Scalar.isScalarValue(_pair.value)) prev.value.value = _pair.value;
				else prev.value = _pair.value;
			} else if (sortEntries) {
				const i = this.items.findIndex((item) => sortEntries(_pair, item) < 0);
				if (i === -1) this.items.push(_pair);
				else this.items.splice(i, 0, _pair);
			} else this.items.push(_pair);
		}
		delete(key) {
			const it = findPair(this.items, key);
			if (!it) return false;
			return this.items.splice(this.items.indexOf(it), 1).length > 0;
		}
		get(key, keepScalar) {
			const node = findPair(this.items, key)?.value;
			return (!keepScalar && identity.isScalar(node) ? node.value : node) ?? void 0;
		}
		has(key) {
			return !!findPair(this.items, key);
		}
		set(key, value) {
			this.add(new Pair.Pair(key, value), true);
		}
		/**
		* @param ctx - Conversion context, originally set in Document#toJS()
		* @param {Class} Type - If set, forces the returned collection type
		* @returns Instance of Type, Map, or Object
		*/
		toJSON(_, ctx, Type) {
			const map = Type ? new Type() : ctx?.mapAsMap ? /* @__PURE__ */ new Map() : {};
			if (ctx?.onCreate) ctx.onCreate(map);
			for (const item of this.items) addPairToJSMap.addPairToJSMap(ctx, map, item);
			return map;
		}
		toString(ctx, onComment, onChompKeep) {
			if (!ctx) return JSON.stringify(this);
			for (const item of this.items) if (!identity.isPair(item)) throw new Error(`Map items must all be pairs; found ${JSON.stringify(item)} instead`);
			if (!ctx.allNullValues && this.hasAllNullValues(false)) ctx = Object.assign({}, ctx, { allNullValues: true });
			return stringifyCollection.stringifyCollection(this, ctx, {
				blockItemPrefix: "",
				flowChars: {
					start: "{",
					end: "}"
				},
				itemIndent: ctx.indent || "",
				onChompKeep,
				onComment
			});
		}
	};
	exports.YAMLMap = YAMLMap;
	exports.findPair = findPair;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/common/map.js
var require_map = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var YAMLMap = require_YAMLMap();
	exports.map = {
		collection: "map",
		default: true,
		nodeClass: YAMLMap.YAMLMap,
		tag: "tag:yaml.org,2002:map",
		resolve(map, onError) {
			if (!identity.isMap(map)) onError("Expected a mapping for this tag");
			return map;
		},
		createNode: (schema, obj, ctx) => YAMLMap.YAMLMap.from(schema, obj, ctx)
	};
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/nodes/YAMLSeq.js
var require_YAMLSeq = /* @__PURE__ */ __commonJSMin(((exports) => {
	var createNode = require_createNode();
	var stringifyCollection = require_stringifyCollection();
	var Collection = require_Collection();
	var identity = require_identity();
	var Scalar = require_Scalar();
	var toJS = require_toJS();
	var YAMLSeq = class extends Collection.Collection {
		static get tagName() {
			return "tag:yaml.org,2002:seq";
		}
		constructor(schema) {
			super(identity.SEQ, schema);
			this.items = [];
		}
		add(value) {
			this.items.push(value);
		}
		/**
		* Removes a value from the collection.
		*
		* `key` must contain a representation of an integer for this to succeed.
		* It may be wrapped in a `Scalar`.
		*
		* @returns `true` if the item was found and removed.
		*/
		delete(key) {
			const idx = asItemIndex(key);
			if (typeof idx !== "number") return false;
			return this.items.splice(idx, 1).length > 0;
		}
		get(key, keepScalar) {
			const idx = asItemIndex(key);
			if (typeof idx !== "number") return void 0;
			const it = this.items[idx];
			return !keepScalar && identity.isScalar(it) ? it.value : it;
		}
		/**
		* Checks if the collection includes a value with the key `key`.
		*
		* `key` must contain a representation of an integer for this to succeed.
		* It may be wrapped in a `Scalar`.
		*/
		has(key) {
			const idx = asItemIndex(key);
			return typeof idx === "number" && idx < this.items.length;
		}
		/**
		* Sets a value in this collection. For `!!set`, `value` needs to be a
		* boolean to add/remove the item from the set.
		*
		* If `key` does not contain a representation of an integer, this will throw.
		* It may be wrapped in a `Scalar`.
		*/
		set(key, value) {
			const idx = asItemIndex(key);
			if (typeof idx !== "number") throw new Error(`Expected a valid index, not ${key}.`);
			const prev = this.items[idx];
			if (identity.isScalar(prev) && Scalar.isScalarValue(value)) prev.value = value;
			else this.items[idx] = value;
		}
		toJSON(_, ctx) {
			const seq = [];
			if (ctx?.onCreate) ctx.onCreate(seq);
			let i = 0;
			for (const item of this.items) seq.push(toJS.toJS(item, String(i++), ctx));
			return seq;
		}
		toString(ctx, onComment, onChompKeep) {
			if (!ctx) return JSON.stringify(this);
			return stringifyCollection.stringifyCollection(this, ctx, {
				blockItemPrefix: "- ",
				flowChars: {
					start: "[",
					end: "]"
				},
				itemIndent: (ctx.indent || "") + "  ",
				onChompKeep,
				onComment
			});
		}
		static from(schema, obj, ctx) {
			const { replacer } = ctx;
			const seq = new this(schema);
			if (obj && Symbol.iterator in Object(obj)) {
				let i = 0;
				for (let it of obj) {
					if (typeof replacer === "function") {
						const key = obj instanceof Set ? it : String(i++);
						it = replacer.call(obj, key, it);
					}
					seq.items.push(createNode.createNode(it, void 0, ctx));
				}
			}
			return seq;
		}
	};
	function asItemIndex(key) {
		let idx = identity.isScalar(key) ? key.value : key;
		if (idx && typeof idx === "string") idx = Number(idx);
		return typeof idx === "number" && Number.isInteger(idx) && idx >= 0 ? idx : null;
	}
	exports.YAMLSeq = YAMLSeq;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/common/seq.js
var require_seq = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var YAMLSeq = require_YAMLSeq();
	exports.seq = {
		collection: "seq",
		default: true,
		nodeClass: YAMLSeq.YAMLSeq,
		tag: "tag:yaml.org,2002:seq",
		resolve(seq, onError) {
			if (!identity.isSeq(seq)) onError("Expected a sequence for this tag");
			return seq;
		},
		createNode: (schema, obj, ctx) => YAMLSeq.YAMLSeq.from(schema, obj, ctx)
	};
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/common/string.js
var require_string = /* @__PURE__ */ __commonJSMin(((exports) => {
	var stringifyString = require_stringifyString();
	exports.string = {
		identify: (value) => typeof value === "string",
		default: true,
		tag: "tag:yaml.org,2002:str",
		resolve: (str) => str,
		stringify(item, ctx, onComment, onChompKeep) {
			ctx = Object.assign({ actualString: true }, ctx);
			return stringifyString.stringifyString(item, ctx, onComment, onChompKeep);
		}
	};
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/common/null.js
var require_null = /* @__PURE__ */ __commonJSMin(((exports) => {
	var Scalar = require_Scalar();
	const nullTag = {
		identify: (value) => value == null,
		createNode: () => new Scalar.Scalar(null),
		default: true,
		tag: "tag:yaml.org,2002:null",
		test: /^(?:~|[Nn]ull|NULL)?$/,
		resolve: () => new Scalar.Scalar(null),
		stringify: ({ source }, ctx) => typeof source === "string" && nullTag.test.test(source) ? source : ctx.options.nullStr
	};
	exports.nullTag = nullTag;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/core/bool.js
var require_bool$1 = /* @__PURE__ */ __commonJSMin(((exports) => {
	var Scalar = require_Scalar();
	const boolTag = {
		identify: (value) => typeof value === "boolean",
		default: true,
		tag: "tag:yaml.org,2002:bool",
		test: /^(?:[Tt]rue|TRUE|[Ff]alse|FALSE)$/,
		resolve: (str) => new Scalar.Scalar(str[0] === "t" || str[0] === "T"),
		stringify({ source, value }, ctx) {
			if (source && boolTag.test.test(source)) {
				if (value === (source[0] === "t" || source[0] === "T")) return source;
			}
			return value ? ctx.options.trueStr : ctx.options.falseStr;
		}
	};
	exports.boolTag = boolTag;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/stringify/stringifyNumber.js
var require_stringifyNumber = /* @__PURE__ */ __commonJSMin(((exports) => {
	function stringifyNumber({ format, minFractionDigits, tag, value }) {
		if (typeof value === "bigint") return String(value);
		const num = typeof value === "number" ? value : Number(value);
		if (!isFinite(num)) return isNaN(num) ? ".nan" : num < 0 ? "-.inf" : ".inf";
		let n = Object.is(value, -0) ? "-0" : JSON.stringify(value);
		if (!format && minFractionDigits && (!tag || tag === "tag:yaml.org,2002:float") && /^-?\d/.test(n) && !n.includes("e")) {
			let i = n.indexOf(".");
			if (i < 0) {
				i = n.length;
				n += ".";
			}
			let d = minFractionDigits - (n.length - i - 1);
			while (d-- > 0) n += "0";
		}
		return n;
	}
	exports.stringifyNumber = stringifyNumber;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/core/float.js
var require_float$1 = /* @__PURE__ */ __commonJSMin(((exports) => {
	var Scalar = require_Scalar();
	var stringifyNumber = require_stringifyNumber();
	const floatNaN = {
		identify: (value) => typeof value === "number",
		default: true,
		tag: "tag:yaml.org,2002:float",
		test: /^(?:[-+]?\.(?:inf|Inf|INF)|\.nan|\.NaN|\.NAN)$/,
		resolve: (str) => str.slice(-3).toLowerCase() === "nan" ? NaN : str[0] === "-" ? Number.NEGATIVE_INFINITY : Number.POSITIVE_INFINITY,
		stringify: stringifyNumber.stringifyNumber
	};
	const floatExp = {
		identify: (value) => typeof value === "number",
		default: true,
		tag: "tag:yaml.org,2002:float",
		format: "EXP",
		test: /^[-+]?(?:\.[0-9]+|[0-9]+(?:\.[0-9]*)?)[eE][-+]?[0-9]+$/,
		resolve: (str) => parseFloat(str),
		stringify(node) {
			const num = Number(node.value);
			return isFinite(num) ? num.toExponential() : stringifyNumber.stringifyNumber(node);
		}
	};
	exports.float = {
		identify: (value) => typeof value === "number",
		default: true,
		tag: "tag:yaml.org,2002:float",
		test: /^[-+]?(?:\.[0-9]+|[0-9]+\.[0-9]*)$/,
		resolve(str) {
			const node = new Scalar.Scalar(parseFloat(str));
			const dot = str.indexOf(".");
			if (dot !== -1 && str[str.length - 1] === "0") node.minFractionDigits = str.length - dot - 1;
			return node;
		},
		stringify: stringifyNumber.stringifyNumber
	};
	exports.floatExp = floatExp;
	exports.floatNaN = floatNaN;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/core/int.js
var require_int$1 = /* @__PURE__ */ __commonJSMin(((exports) => {
	var stringifyNumber = require_stringifyNumber();
	const intIdentify = (value) => typeof value === "bigint" || Number.isInteger(value);
	const intResolve = (str, offset, radix, { intAsBigInt }) => intAsBigInt ? BigInt(str) : parseInt(str.substring(offset), radix);
	function intStringify(node, radix, prefix) {
		const { value } = node;
		if (intIdentify(value) && value >= 0) return prefix + value.toString(radix);
		return stringifyNumber.stringifyNumber(node);
	}
	const intOct = {
		identify: (value) => intIdentify(value) && value >= 0,
		default: true,
		tag: "tag:yaml.org,2002:int",
		format: "OCT",
		test: /^0o[0-7]+$/,
		resolve: (str, _onError, opt) => intResolve(str, 2, 8, opt),
		stringify: (node) => intStringify(node, 8, "0o")
	};
	const int = {
		identify: intIdentify,
		default: true,
		tag: "tag:yaml.org,2002:int",
		test: /^[-+]?[0-9]+$/,
		resolve: (str, _onError, opt) => intResolve(str, 0, 10, opt),
		stringify: stringifyNumber.stringifyNumber
	};
	const intHex = {
		identify: (value) => intIdentify(value) && value >= 0,
		default: true,
		tag: "tag:yaml.org,2002:int",
		format: "HEX",
		test: /^0x[0-9a-fA-F]+$/,
		resolve: (str, _onError, opt) => intResolve(str, 2, 16, opt),
		stringify: (node) => intStringify(node, 16, "0x")
	};
	exports.int = int;
	exports.intHex = intHex;
	exports.intOct = intOct;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/core/schema.js
var require_schema$2 = /* @__PURE__ */ __commonJSMin(((exports) => {
	var map = require_map();
	var _null = require_null();
	var seq = require_seq();
	var string = require_string();
	var bool = require_bool$1();
	var float = require_float$1();
	var int = require_int$1();
	exports.schema = [
		map.map,
		seq.seq,
		string.string,
		_null.nullTag,
		bool.boolTag,
		int.intOct,
		int.int,
		int.intHex,
		float.floatNaN,
		float.floatExp,
		float.float
	];
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/json/schema.js
var require_schema$1 = /* @__PURE__ */ __commonJSMin(((exports) => {
	var Scalar = require_Scalar();
	var map = require_map();
	var seq = require_seq();
	function intIdentify(value) {
		return typeof value === "bigint" || Number.isInteger(value);
	}
	const stringifyJSON = ({ value }) => JSON.stringify(value);
	const jsonScalars = [
		{
			identify: (value) => typeof value === "string",
			default: true,
			tag: "tag:yaml.org,2002:str",
			resolve: (str) => str,
			stringify: stringifyJSON
		},
		{
			identify: (value) => value == null,
			createNode: () => new Scalar.Scalar(null),
			default: true,
			tag: "tag:yaml.org,2002:null",
			test: /^null$/,
			resolve: () => null,
			stringify: stringifyJSON
		},
		{
			identify: (value) => typeof value === "boolean",
			default: true,
			tag: "tag:yaml.org,2002:bool",
			test: /^true$|^false$/,
			resolve: (str) => str === "true",
			stringify: stringifyJSON
		},
		{
			identify: intIdentify,
			default: true,
			tag: "tag:yaml.org,2002:int",
			test: /^-?(?:0|[1-9][0-9]*)$/,
			resolve: (str, _onError, { intAsBigInt }) => intAsBigInt ? BigInt(str) : parseInt(str, 10),
			stringify: ({ value }) => intIdentify(value) ? value.toString() : JSON.stringify(value)
		},
		{
			identify: (value) => typeof value === "number",
			default: true,
			tag: "tag:yaml.org,2002:float",
			test: /^-?(?:0|[1-9][0-9]*)(?:\.[0-9]*)?(?:[eE][-+]?[0-9]+)?$/,
			resolve: (str) => parseFloat(str),
			stringify: stringifyJSON
		}
	];
	exports.schema = [map.map, seq.seq].concat(jsonScalars, {
		default: true,
		tag: "",
		test: /^/,
		resolve(str, onError) {
			onError(`Unresolved plain scalar ${JSON.stringify(str)}`);
			return str;
		}
	});
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/yaml-1.1/binary.js
var require_binary = /* @__PURE__ */ __commonJSMin(((exports) => {
	var node_buffer = __require("buffer");
	var Scalar = require_Scalar();
	var stringifyString = require_stringifyString();
	exports.binary = {
		identify: (value) => value instanceof Uint8Array,
		default: false,
		tag: "tag:yaml.org,2002:binary",
		/**
		* Returns a Buffer in node and an Uint8Array in browsers
		*
		* To use the resulting buffer as an image, you'll want to do something like:
		*
		*   const blob = new Blob([buffer], { type: 'image/jpeg' })
		*   document.querySelector('#photo').src = URL.createObjectURL(blob)
		*/
		resolve(src, onError) {
			if (typeof node_buffer.Buffer === "function") return node_buffer.Buffer.from(src, "base64");
			else if (typeof atob === "function") {
				const str = atob(src.replace(/[\n\r]/g, ""));
				const buffer = new Uint8Array(str.length);
				for (let i = 0; i < str.length; ++i) buffer[i] = str.charCodeAt(i);
				return buffer;
			} else {
				onError("This environment does not support reading binary tags; either Buffer or atob is required");
				return src;
			}
		},
		stringify({ comment, type, value }, ctx, onComment, onChompKeep) {
			if (!value) return "";
			const buf = value;
			let str;
			if (typeof node_buffer.Buffer === "function") str = buf instanceof node_buffer.Buffer ? buf.toString("base64") : node_buffer.Buffer.from(buf.buffer).toString("base64");
			else if (typeof btoa === "function") {
				let s = "";
				for (let i = 0; i < buf.length; ++i) s += String.fromCharCode(buf[i]);
				str = btoa(s);
			} else throw new Error("This environment does not support writing binary tags; either Buffer or btoa is required");
			type ?? (type = Scalar.Scalar.BLOCK_LITERAL);
			if (type !== Scalar.Scalar.QUOTE_DOUBLE) {
				const lineWidth = Math.max(ctx.options.lineWidth - ctx.indent.length, ctx.options.minContentWidth);
				const n = Math.ceil(str.length / lineWidth);
				const lines = new Array(n);
				for (let i = 0, o = 0; i < n; ++i, o += lineWidth) lines[i] = str.substr(o, lineWidth);
				str = lines.join(type === Scalar.Scalar.BLOCK_LITERAL ? "\n" : " ");
			}
			return stringifyString.stringifyString({
				comment,
				type,
				value: str
			}, ctx, onComment, onChompKeep);
		}
	};
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/yaml-1.1/pairs.js
var require_pairs = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var Pair = require_Pair();
	var Scalar = require_Scalar();
	var YAMLSeq = require_YAMLSeq();
	function resolvePairs(seq, onError) {
		if (identity.isSeq(seq)) for (let i = 0; i < seq.items.length; ++i) {
			let item = seq.items[i];
			if (identity.isPair(item)) continue;
			else if (identity.isMap(item)) {
				if (item.items.length > 1) onError("Each pair must have its own sequence indicator");
				const pair = item.items[0] || new Pair.Pair(new Scalar.Scalar(null));
				if (item.commentBefore) pair.key.commentBefore = pair.key.commentBefore ? `${item.commentBefore}\n${pair.key.commentBefore}` : item.commentBefore;
				if (item.comment) {
					const cn = pair.value ?? pair.key;
					cn.comment = cn.comment ? `${item.comment}\n${cn.comment}` : item.comment;
				}
				item = pair;
			}
			seq.items[i] = identity.isPair(item) ? item : new Pair.Pair(item);
		}
		else onError("Expected a sequence for this tag");
		return seq;
	}
	function createPairs(schema, iterable, ctx) {
		const { replacer } = ctx;
		const pairs = new YAMLSeq.YAMLSeq(schema);
		pairs.tag = "tag:yaml.org,2002:pairs";
		let i = 0;
		if (iterable && Symbol.iterator in Object(iterable)) for (let it of iterable) {
			if (typeof replacer === "function") it = replacer.call(iterable, String(i++), it);
			let key, value;
			if (Array.isArray(it)) {
				if (it.length === 2) {
					key = it[0];
					value = it[1];
				} else throw new TypeError(`Expected [key, value] tuple: ${it}`);
			} else if (it && it instanceof Object) {
				const keys = Object.keys(it);
				if (keys.length === 1) {
					key = keys[0];
					value = it[key];
				} else throw new TypeError(`Expected tuple with one key, not ${keys.length} keys`);
			} else key = it;
			pairs.items.push(Pair.createPair(key, value, ctx));
		}
		return pairs;
	}
	const pairs = {
		collection: "seq",
		default: false,
		tag: "tag:yaml.org,2002:pairs",
		resolve: resolvePairs,
		createNode: createPairs
	};
	exports.createPairs = createPairs;
	exports.pairs = pairs;
	exports.resolvePairs = resolvePairs;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/yaml-1.1/omap.js
var require_omap = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var toJS = require_toJS();
	var YAMLMap = require_YAMLMap();
	var YAMLSeq = require_YAMLSeq();
	var pairs = require_pairs();
	var YAMLOMap = class YAMLOMap extends YAMLSeq.YAMLSeq {
		constructor() {
			super();
			this.add = YAMLMap.YAMLMap.prototype.add.bind(this);
			this.delete = YAMLMap.YAMLMap.prototype.delete.bind(this);
			this.get = YAMLMap.YAMLMap.prototype.get.bind(this);
			this.has = YAMLMap.YAMLMap.prototype.has.bind(this);
			this.set = YAMLMap.YAMLMap.prototype.set.bind(this);
			this.tag = YAMLOMap.tag;
		}
		/**
		* If `ctx` is given, the return type is actually `Map<unknown, unknown>`,
		* but TypeScript won't allow widening the signature of a child method.
		*/
		toJSON(_, ctx) {
			if (!ctx) return super.toJSON(_);
			const map = /* @__PURE__ */ new Map();
			if (ctx?.onCreate) ctx.onCreate(map);
			for (const pair of this.items) {
				let key, value;
				if (identity.isPair(pair)) {
					key = toJS.toJS(pair.key, "", ctx);
					value = toJS.toJS(pair.value, key, ctx);
				} else key = toJS.toJS(pair, "", ctx);
				if (map.has(key)) throw new Error("Ordered maps must not include duplicate keys");
				map.set(key, value);
			}
			return map;
		}
		static from(schema, iterable, ctx) {
			const pairs$1 = pairs.createPairs(schema, iterable, ctx);
			const omap = new this();
			omap.items = pairs$1.items;
			return omap;
		}
	};
	YAMLOMap.tag = "tag:yaml.org,2002:omap";
	const omap = {
		collection: "seq",
		identify: (value) => value instanceof Map,
		nodeClass: YAMLOMap,
		default: false,
		tag: "tag:yaml.org,2002:omap",
		resolve(seq, onError) {
			const pairs$1 = pairs.resolvePairs(seq, onError);
			const seenKeys = [];
			for (const { key } of pairs$1.items) if (identity.isScalar(key)) {
				if (seenKeys.includes(key.value)) onError(`Ordered maps must not include duplicate keys: ${key.value}`);
				else seenKeys.push(key.value);
			}
			return Object.assign(new YAMLOMap(), pairs$1);
		},
		createNode: (schema, iterable, ctx) => YAMLOMap.from(schema, iterable, ctx)
	};
	exports.YAMLOMap = YAMLOMap;
	exports.omap = omap;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/yaml-1.1/bool.js
var require_bool = /* @__PURE__ */ __commonJSMin(((exports) => {
	var Scalar = require_Scalar();
	function boolStringify({ value, source }, ctx) {
		if (source && (value ? trueTag : falseTag).test.test(source)) return source;
		return value ? ctx.options.trueStr : ctx.options.falseStr;
	}
	const trueTag = {
		identify: (value) => value === true,
		default: true,
		tag: "tag:yaml.org,2002:bool",
		test: /^(?:Y|y|[Yy]es|YES|[Tt]rue|TRUE|[Oo]n|ON)$/,
		resolve: () => new Scalar.Scalar(true),
		stringify: boolStringify
	};
	const falseTag = {
		identify: (value) => value === false,
		default: true,
		tag: "tag:yaml.org,2002:bool",
		test: /^(?:N|n|[Nn]o|NO|[Ff]alse|FALSE|[Oo]ff|OFF)$/,
		resolve: () => new Scalar.Scalar(false),
		stringify: boolStringify
	};
	exports.falseTag = falseTag;
	exports.trueTag = trueTag;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/yaml-1.1/float.js
var require_float = /* @__PURE__ */ __commonJSMin(((exports) => {
	var Scalar = require_Scalar();
	var stringifyNumber = require_stringifyNumber();
	const floatNaN = {
		identify: (value) => typeof value === "number",
		default: true,
		tag: "tag:yaml.org,2002:float",
		test: /^(?:[-+]?\.(?:inf|Inf|INF)|\.nan|\.NaN|\.NAN)$/,
		resolve: (str) => str.slice(-3).toLowerCase() === "nan" ? NaN : str[0] === "-" ? Number.NEGATIVE_INFINITY : Number.POSITIVE_INFINITY,
		stringify: stringifyNumber.stringifyNumber
	};
	const floatExp = {
		identify: (value) => typeof value === "number",
		default: true,
		tag: "tag:yaml.org,2002:float",
		format: "EXP",
		test: /^[-+]?(?:[0-9][0-9_]*)?(?:\.[0-9_]*)?[eE][-+]?[0-9]+$/,
		resolve: (str) => parseFloat(str.replace(/_/g, "")),
		stringify(node) {
			const num = Number(node.value);
			return isFinite(num) ? num.toExponential() : stringifyNumber.stringifyNumber(node);
		}
	};
	exports.float = {
		identify: (value) => typeof value === "number",
		default: true,
		tag: "tag:yaml.org,2002:float",
		test: /^[-+]?(?:[0-9][0-9_]*)?\.[0-9_]*$/,
		resolve(str) {
			const node = new Scalar.Scalar(parseFloat(str.replace(/_/g, "")));
			const dot = str.indexOf(".");
			if (dot !== -1) {
				const f = str.substring(dot + 1).replace(/_/g, "");
				if (f[f.length - 1] === "0") node.minFractionDigits = f.length;
			}
			return node;
		},
		stringify: stringifyNumber.stringifyNumber
	};
	exports.floatExp = floatExp;
	exports.floatNaN = floatNaN;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/yaml-1.1/int.js
var require_int = /* @__PURE__ */ __commonJSMin(((exports) => {
	var stringifyNumber = require_stringifyNumber();
	const intIdentify = (value) => typeof value === "bigint" || Number.isInteger(value);
	function intResolve(str, offset, radix, { intAsBigInt }) {
		const sign = str[0];
		if (sign === "-" || sign === "+") offset += 1;
		str = str.substring(offset).replace(/_/g, "");
		if (intAsBigInt) {
			switch (radix) {
				case 2:
					str = `0b${str}`;
					break;
				case 8:
					str = `0o${str}`;
					break;
				case 16: str = `0x${str}`;
			}
			const n = BigInt(str);
			return sign === "-" ? BigInt(-1) * n : n;
		}
		const n = parseInt(str, radix);
		return sign === "-" ? -1 * n : n;
	}
	function intStringify(node, radix, prefix) {
		const { value } = node;
		if (intIdentify(value)) {
			const str = value.toString(radix);
			return value < 0 ? "-" + prefix + str.substr(1) : prefix + str;
		}
		return stringifyNumber.stringifyNumber(node);
	}
	const intBin = {
		identify: intIdentify,
		default: true,
		tag: "tag:yaml.org,2002:int",
		format: "BIN",
		test: /^[-+]?0b[0-1_]+$/,
		resolve: (str, _onError, opt) => intResolve(str, 2, 2, opt),
		stringify: (node) => intStringify(node, 2, "0b")
	};
	const intOct = {
		identify: intIdentify,
		default: true,
		tag: "tag:yaml.org,2002:int",
		format: "OCT",
		test: /^[-+]?0[0-7_]+$/,
		resolve: (str, _onError, opt) => intResolve(str, 1, 8, opt),
		stringify: (node) => intStringify(node, 8, "0")
	};
	const int = {
		identify: intIdentify,
		default: true,
		tag: "tag:yaml.org,2002:int",
		test: /^[-+]?[0-9][0-9_]*$/,
		resolve: (str, _onError, opt) => intResolve(str, 0, 10, opt),
		stringify: stringifyNumber.stringifyNumber
	};
	const intHex = {
		identify: intIdentify,
		default: true,
		tag: "tag:yaml.org,2002:int",
		format: "HEX",
		test: /^[-+]?0x[0-9a-fA-F_]+$/,
		resolve: (str, _onError, opt) => intResolve(str, 2, 16, opt),
		stringify: (node) => intStringify(node, 16, "0x")
	};
	exports.int = int;
	exports.intBin = intBin;
	exports.intHex = intHex;
	exports.intOct = intOct;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/yaml-1.1/set.js
var require_set = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var Pair = require_Pair();
	var YAMLMap = require_YAMLMap();
	var YAMLSet = class YAMLSet extends YAMLMap.YAMLMap {
		constructor(schema) {
			super(schema);
			this.tag = YAMLSet.tag;
		}
		add(key) {
			let pair;
			if (identity.isPair(key)) pair = key;
			else if (key && typeof key === "object" && "key" in key && "value" in key && key.value === null) pair = new Pair.Pair(key.key, null);
			else pair = new Pair.Pair(key, null);
			if (!YAMLMap.findPair(this.items, pair.key)) this.items.push(pair);
		}
		/**
		* If `keepPair` is `true`, returns the Pair matching `key`.
		* Otherwise, returns the value of that Pair's key.
		*/
		get(key, keepPair) {
			const pair = YAMLMap.findPair(this.items, key);
			return !keepPair && identity.isPair(pair) ? identity.isScalar(pair.key) ? pair.key.value : pair.key : pair;
		}
		set(key, value) {
			if (typeof value !== "boolean") throw new Error(`Expected boolean value for set(key, value) in a YAML set, not ${typeof value}`);
			const prev = YAMLMap.findPair(this.items, key);
			if (prev && !value) this.items.splice(this.items.indexOf(prev), 1);
			else if (!prev && value) this.items.push(new Pair.Pair(key));
		}
		toJSON(_, ctx) {
			return super.toJSON(_, ctx, Set);
		}
		toString(ctx, onComment, onChompKeep) {
			if (!ctx) return JSON.stringify(this);
			if (this.hasAllNullValues(true)) return super.toString(Object.assign({}, ctx, { allNullValues: true }), onComment, onChompKeep);
			else throw new Error("Set items must all have null values");
		}
		static from(schema, iterable, ctx) {
			const { replacer } = ctx;
			const set = new this(schema);
			if (iterable && Symbol.iterator in Object(iterable)) for (let value of iterable) {
				if (typeof replacer === "function") value = replacer.call(iterable, value, value);
				set.items.push(Pair.createPair(value, null, ctx));
			}
			return set;
		}
	};
	YAMLSet.tag = "tag:yaml.org,2002:set";
	const set = {
		collection: "map",
		identify: (value) => value instanceof Set,
		nodeClass: YAMLSet,
		default: false,
		tag: "tag:yaml.org,2002:set",
		createNode: (schema, iterable, ctx) => YAMLSet.from(schema, iterable, ctx),
		resolve(map, onError) {
			if (identity.isMap(map)) {
				if (map.hasAllNullValues(true)) return Object.assign(new YAMLSet(), map);
				else onError("Set items must all have null values");
			} else onError("Expected a mapping for this tag");
			return map;
		}
	};
	exports.YAMLSet = YAMLSet;
	exports.set = set;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/yaml-1.1/timestamp.js
var require_timestamp = /* @__PURE__ */ __commonJSMin(((exports) => {
	var stringifyNumber = require_stringifyNumber();
	/** Internal types handle bigint as number, because TS can't figure it out. */
	function parseSexagesimal(str, asBigInt) {
		const sign = str[0];
		const parts = sign === "-" || sign === "+" ? str.substring(1) : str;
		const num = (n) => asBigInt ? BigInt(n) : Number(n);
		const res = parts.replace(/_/g, "").split(":").reduce((res, p) => res * num(60) + num(p), num(0));
		return sign === "-" ? num(-1) * res : res;
	}
	/**
	* hhhh:mm:ss.sss
	*
	* Internal types handle bigint as number, because TS can't figure it out.
	*/
	function stringifySexagesimal(node) {
		let { value } = node;
		let num = (n) => n;
		if (typeof value === "bigint") num = (n) => BigInt(n);
		else if (isNaN(value) || !isFinite(value)) return stringifyNumber.stringifyNumber(node);
		let sign = "";
		if (value < 0) {
			sign = "-";
			value *= num(-1);
		}
		const _60 = num(60);
		const parts = [value % _60];
		if (value < 60) parts.unshift(0);
		else {
			value = (value - parts[0]) / _60;
			parts.unshift(value % _60);
			if (value >= 60) {
				value = (value - parts[0]) / _60;
				parts.unshift(value);
			}
		}
		return sign + parts.map((n) => String(n).padStart(2, "0")).join(":").replace(/000000\d*$/, "");
	}
	const intTime = {
		identify: (value) => typeof value === "bigint" || Number.isInteger(value),
		default: true,
		tag: "tag:yaml.org,2002:int",
		format: "TIME",
		test: /^[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+$/,
		resolve: (str, _onError, { intAsBigInt }) => parseSexagesimal(str, intAsBigInt),
		stringify: stringifySexagesimal
	};
	const floatTime = {
		identify: (value) => typeof value === "number",
		default: true,
		tag: "tag:yaml.org,2002:float",
		format: "TIME",
		test: /^[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+\.[0-9_]*$/,
		resolve: (str) => parseSexagesimal(str, false),
		stringify: stringifySexagesimal
	};
	const timestamp = {
		identify: (value) => value instanceof Date,
		default: true,
		tag: "tag:yaml.org,2002:timestamp",
		test: RegExp("^([0-9]{4})-([0-9]{1,2})-([0-9]{1,2})(?:(?:t|T|[ \\t]+)([0-9]{1,2}):([0-9]{1,2}):([0-9]{1,2}(\\.[0-9]+)?)(?:[ \\t]*(Z|[-+][012]?[0-9](?::[0-9]{2})?))?)?$"),
		resolve(str) {
			const match = str.match(timestamp.test);
			if (!match) throw new Error("!!timestamp expects a date, starting with yyyy-mm-dd");
			const [, year, month, day, hour, minute, second] = match.map(Number);
			const millisec = match[7] ? Number((match[7] + "00").substr(1, 3)) : 0;
			let date = Date.UTC(year, month - 1, day, hour || 0, minute || 0, second || 0, millisec);
			const tz = match[8];
			if (tz && tz !== "Z") {
				let d = parseSexagesimal(tz, false);
				if (Math.abs(d) < 30) d *= 60;
				date -= 6e4 * d;
			}
			return new Date(date);
		},
		stringify: ({ value }) => value?.toISOString().replace(/(T00:00:00)?\.000Z$/, "") ?? ""
	};
	exports.floatTime = floatTime;
	exports.intTime = intTime;
	exports.timestamp = timestamp;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/yaml-1.1/schema.js
var require_schema = /* @__PURE__ */ __commonJSMin(((exports) => {
	var map = require_map();
	var _null = require_null();
	var seq = require_seq();
	var string = require_string();
	var binary = require_binary();
	var bool = require_bool();
	var float = require_float();
	var int = require_int();
	var merge = require_merge();
	var omap = require_omap();
	var pairs = require_pairs();
	var set = require_set();
	var timestamp = require_timestamp();
	exports.schema = [
		map.map,
		seq.seq,
		string.string,
		_null.nullTag,
		bool.trueTag,
		bool.falseTag,
		int.intBin,
		int.intOct,
		int.int,
		int.intHex,
		float.floatNaN,
		float.floatExp,
		float.float,
		binary.binary,
		merge.merge,
		omap.omap,
		pairs.pairs,
		set.set,
		timestamp.intTime,
		timestamp.floatTime,
		timestamp.timestamp
	];
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/tags.js
var require_tags = /* @__PURE__ */ __commonJSMin(((exports) => {
	var map = require_map();
	var _null = require_null();
	var seq = require_seq();
	var string = require_string();
	var bool = require_bool$1();
	var float = require_float$1();
	var int = require_int$1();
	var schema = require_schema$2();
	var schema$1 = require_schema$1();
	var binary = require_binary();
	var merge = require_merge();
	var omap = require_omap();
	var pairs = require_pairs();
	var schema$2 = require_schema();
	var set = require_set();
	var timestamp = require_timestamp();
	const schemas = /* @__PURE__ */ new Map([
		["core", schema.schema],
		["failsafe", [
			map.map,
			seq.seq,
			string.string
		]],
		["json", schema$1.schema],
		["yaml11", schema$2.schema],
		["yaml-1.1", schema$2.schema]
	]);
	const tagsByName = {
		binary: binary.binary,
		bool: bool.boolTag,
		float: float.float,
		floatExp: float.floatExp,
		floatNaN: float.floatNaN,
		floatTime: timestamp.floatTime,
		int: int.int,
		intHex: int.intHex,
		intOct: int.intOct,
		intTime: timestamp.intTime,
		map: map.map,
		merge: merge.merge,
		null: _null.nullTag,
		omap: omap.omap,
		pairs: pairs.pairs,
		seq: seq.seq,
		set: set.set,
		timestamp: timestamp.timestamp
	};
	const coreKnownTags = {
		"tag:yaml.org,2002:binary": binary.binary,
		"tag:yaml.org,2002:merge": merge.merge,
		"tag:yaml.org,2002:omap": omap.omap,
		"tag:yaml.org,2002:pairs": pairs.pairs,
		"tag:yaml.org,2002:set": set.set,
		"tag:yaml.org,2002:timestamp": timestamp.timestamp
	};
	function getTags(customTags, schemaName, addMergeTag) {
		const schemaTags = schemas.get(schemaName);
		if (schemaTags && !customTags) return addMergeTag && !schemaTags.includes(merge.merge) ? schemaTags.concat(merge.merge) : schemaTags.slice();
		let tags = schemaTags;
		if (!tags) {
			if (Array.isArray(customTags)) tags = [];
			else {
				const keys = Array.from(schemas.keys()).filter((key) => key !== "yaml11").map((key) => JSON.stringify(key)).join(", ");
				throw new Error(`Unknown schema "${schemaName}"; use one of ${keys} or define customTags array`);
			}
		}
		if (Array.isArray(customTags)) for (const tag of customTags) tags = tags.concat(tag);
		else if (typeof customTags === "function") tags = customTags(tags.slice());
		if (addMergeTag) tags = tags.concat(merge.merge);
		return tags.reduce((tags, tag) => {
			const tagObj = typeof tag === "string" ? tagsByName[tag] : tag;
			if (!tagObj) {
				const tagName = JSON.stringify(tag);
				const keys = Object.keys(tagsByName).map((key) => JSON.stringify(key)).join(", ");
				throw new Error(`Unknown custom tag ${tagName}; use one of ${keys}`);
			}
			if (!tags.includes(tagObj)) tags.push(tagObj);
			return tags;
		}, []);
	}
	exports.coreKnownTags = coreKnownTags;
	exports.getTags = getTags;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/schema/Schema.js
var require_Schema = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var map = require_map();
	var seq = require_seq();
	var string = require_string();
	var tags = require_tags();
	const sortMapEntriesByKey = (a, b) => a.key < b.key ? -1 : a.key > b.key ? 1 : 0;
	exports.Schema = class Schema {
		constructor({ compat, customTags, merge, resolveKnownTags, schema, sortMapEntries, toStringDefaults }) {
			this.compat = Array.isArray(compat) ? tags.getTags(compat, "compat") : compat ? tags.getTags(null, compat) : null;
			this.name = typeof schema === "string" && schema || "core";
			this.knownTags = resolveKnownTags ? tags.coreKnownTags : {};
			this.tags = tags.getTags(customTags, this.name, merge);
			this.toStringOptions = toStringDefaults ?? null;
			Object.defineProperty(this, identity.MAP, { value: map.map });
			Object.defineProperty(this, identity.SCALAR, { value: string.string });
			Object.defineProperty(this, identity.SEQ, { value: seq.seq });
			this.sortMapEntries = typeof sortMapEntries === "function" ? sortMapEntries : sortMapEntries === true ? sortMapEntriesByKey : null;
		}
		clone() {
			const copy = Object.create(Schema.prototype, Object.getOwnPropertyDescriptors(this));
			copy.tags = this.tags.slice();
			return copy;
		}
	};
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/stringify/stringifyDocument.js
var require_stringifyDocument = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var stringify = require_stringify();
	var stringifyComment = require_stringifyComment();
	function stringifyDocument(doc, options) {
		const lines = [];
		let hasDirectives = options.directives === true;
		if (options.directives !== false && doc.directives) {
			const dir = doc.directives.toString(doc);
			if (dir) {
				lines.push(dir);
				hasDirectives = true;
			} else if (doc.directives.docStart) hasDirectives = true;
		}
		if (hasDirectives) lines.push("---");
		const ctx = stringify.createStringifyContext(doc, options);
		const { commentString } = ctx.options;
		if (doc.commentBefore) {
			if (lines.length !== 1) lines.unshift("");
			const cs = commentString(doc.commentBefore);
			lines.unshift(stringifyComment.indentComment(cs, ""));
		}
		let chompKeep = false;
		let contentComment = null;
		if (doc.contents) {
			if (identity.isNode(doc.contents)) {
				if (doc.contents.spaceBefore && hasDirectives) lines.push("");
				if (doc.contents.commentBefore) {
					const cs = commentString(doc.contents.commentBefore);
					lines.push(stringifyComment.indentComment(cs, ""));
				}
				ctx.forceBlockIndent = !!doc.comment;
				contentComment = doc.contents.comment;
			}
			const onChompKeep = contentComment ? void 0 : () => chompKeep = true;
			let body = stringify.stringify(doc.contents, ctx, () => contentComment = null, onChompKeep);
			if (contentComment) body += stringifyComment.lineComment(body, "", commentString(contentComment));
			if ((body[0] === "|" || body[0] === ">") && lines[lines.length - 1] === "---") lines[lines.length - 1] = `--- ${body}`;
			else lines.push(body);
		} else lines.push(stringify.stringify(doc.contents, ctx));
		if (doc.directives?.docEnd) {
			if (doc.comment) {
				const cs = commentString(doc.comment);
				if (cs.includes("\n")) {
					lines.push("...");
					lines.push(stringifyComment.indentComment(cs, ""));
				} else lines.push(`... ${cs}`);
			} else lines.push("...");
		} else {
			let dc = doc.comment;
			if (dc && chompKeep) dc = dc.replace(/^\n+/, "");
			if (dc) {
				if ((!chompKeep || contentComment) && lines[lines.length - 1] !== "") lines.push("");
				lines.push(stringifyComment.indentComment(commentString(dc), ""));
			}
		}
		return lines.join("\n") + "\n";
	}
	exports.stringifyDocument = stringifyDocument;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/doc/Document.js
var require_Document = /* @__PURE__ */ __commonJSMin(((exports) => {
	var Alias = require_Alias();
	var Collection = require_Collection();
	var identity = require_identity();
	var Pair = require_Pair();
	var toJS = require_toJS();
	var Schema = require_Schema();
	var stringifyDocument = require_stringifyDocument();
	var anchors = require_anchors();
	var applyReviver = require_applyReviver();
	var createNode = require_createNode();
	var directives = require_directives();
	var Document = class Document {
		constructor(value, replacer, options) {
			/** A comment before this Document */
			this.commentBefore = null;
			/** A comment immediately after this Document */
			this.comment = null;
			/** Errors encountered during parsing. */
			this.errors = [];
			/** Warnings encountered during parsing. */
			this.warnings = [];
			Object.defineProperty(this, identity.NODE_TYPE, { value: identity.DOC });
			let _replacer = null;
			if (typeof replacer === "function" || Array.isArray(replacer)) _replacer = replacer;
			else if (options === void 0 && replacer) {
				options = replacer;
				replacer = void 0;
			}
			const opt = Object.assign({
				intAsBigInt: false,
				keepSourceTokens: false,
				logLevel: "warn",
				prettyErrors: true,
				strict: true,
				stringKeys: false,
				uniqueKeys: true,
				version: "1.2"
			}, options);
			this.options = opt;
			let { version } = opt;
			if (options?._directives) {
				this.directives = options._directives.atDocument();
				if (this.directives.yaml.explicit) version = this.directives.yaml.version;
			} else this.directives = new directives.Directives({ version });
			this.setSchema(version, options);
			this.contents = value === void 0 ? null : this.createNode(value, _replacer, options);
		}
		/**
		* Create a deep copy of this Document and its contents.
		*
		* Custom Node values that inherit from `Object` still refer to their original instances.
		*/
		clone() {
			const copy = Object.create(Document.prototype, { [identity.NODE_TYPE]: { value: identity.DOC } });
			copy.commentBefore = this.commentBefore;
			copy.comment = this.comment;
			copy.errors = this.errors.slice();
			copy.warnings = this.warnings.slice();
			copy.options = Object.assign({}, this.options);
			if (this.directives) copy.directives = this.directives.clone();
			copy.schema = this.schema.clone();
			copy.contents = identity.isNode(this.contents) ? this.contents.clone(copy.schema) : this.contents;
			if (this.range) copy.range = this.range.slice();
			return copy;
		}
		/** Adds a value to the document. */
		add(value) {
			if (assertCollection(this.contents)) this.contents.add(value);
		}
		/** Adds a value to the document. */
		addIn(path, value) {
			if (assertCollection(this.contents)) this.contents.addIn(path, value);
		}
		/**
		* Create a new `Alias` node, ensuring that the target `node` has the required anchor.
		*
		* If `node` already has an anchor, `name` is ignored.
		* Otherwise, the `node.anchor` value will be set to `name`,
		* or if an anchor with that name is already present in the document,
		* `name` will be used as a prefix for a new unique anchor.
		* If `name` is undefined, the generated anchor will use 'a' as a prefix.
		*/
		createAlias(node, name) {
			if (!node.anchor) {
				const prev = anchors.anchorNames(this);
				node.anchor = !name || prev.has(name) ? anchors.findNewAnchor(name || "a", prev) : name;
			}
			return new Alias.Alias(node.anchor);
		}
		createNode(value, replacer, options) {
			let _replacer = void 0;
			if (typeof replacer === "function") {
				value = replacer.call({ "": value }, "", value);
				_replacer = replacer;
			} else if (Array.isArray(replacer)) {
				const keyToStr = (v) => typeof v === "number" || v instanceof String || v instanceof Number;
				const asStr = replacer.filter(keyToStr).map(String);
				if (asStr.length > 0) replacer = replacer.concat(asStr);
				_replacer = replacer;
			} else if (options === void 0 && replacer) {
				options = replacer;
				replacer = void 0;
			}
			const { aliasDuplicateObjects, anchorPrefix, flow, keepUndefined, onTagObj, tag } = options ?? {};
			const { onAnchor, setAnchors, sourceObjects } = anchors.createNodeAnchors(this, anchorPrefix || "a");
			const ctx = {
				aliasDuplicateObjects: aliasDuplicateObjects ?? true,
				keepUndefined: keepUndefined ?? false,
				onAnchor,
				onTagObj,
				replacer: _replacer,
				schema: this.schema,
				sourceObjects
			};
			const node = createNode.createNode(value, tag, ctx);
			if (flow && identity.isCollection(node)) node.flow = true;
			setAnchors();
			return node;
		}
		/**
		* Convert a key and a value into a `Pair` using the current schema,
		* recursively wrapping all values as `Scalar` or `Collection` nodes.
		*/
		createPair(key, value, options = {}) {
			const k = this.createNode(key, null, options);
			const v = this.createNode(value, null, options);
			return new Pair.Pair(k, v);
		}
		/**
		* Removes a value from the document.
		* @returns `true` if the item was found and removed.
		*/
		delete(key) {
			return assertCollection(this.contents) ? this.contents.delete(key) : false;
		}
		/**
		* Removes a value from the document.
		* @returns `true` if the item was found and removed.
		*/
		deleteIn(path) {
			if (Collection.isEmptyPath(path)) {
				if (this.contents == null) return false;
				this.contents = null;
				return true;
			}
			return assertCollection(this.contents) ? this.contents.deleteIn(path) : false;
		}
		/**
		* Returns item at `key`, or `undefined` if not found. By default unwraps
		* scalar values from their surrounding node; to disable set `keepScalar` to
		* `true` (collections are always returned intact).
		*/
		get(key, keepScalar) {
			return identity.isCollection(this.contents) ? this.contents.get(key, keepScalar) : void 0;
		}
		/**
		* Returns item at `path`, or `undefined` if not found. By default unwraps
		* scalar values from their surrounding node; to disable set `keepScalar` to
		* `true` (collections are always returned intact).
		*/
		getIn(path, keepScalar) {
			if (Collection.isEmptyPath(path)) return !keepScalar && identity.isScalar(this.contents) ? this.contents.value : this.contents;
			return identity.isCollection(this.contents) ? this.contents.getIn(path, keepScalar) : void 0;
		}
		/**
		* Checks if the document includes a value with the key `key`.
		*/
		has(key) {
			return identity.isCollection(this.contents) ? this.contents.has(key) : false;
		}
		/**
		* Checks if the document includes a value at `path`.
		*/
		hasIn(path) {
			if (Collection.isEmptyPath(path)) return this.contents !== void 0;
			return identity.isCollection(this.contents) ? this.contents.hasIn(path) : false;
		}
		/**
		* Sets a value in this document. For `!!set`, `value` needs to be a
		* boolean to add/remove the item from the set.
		*/
		set(key, value) {
			if (this.contents == null) this.contents = Collection.collectionFromPath(this.schema, [key], value);
			else if (assertCollection(this.contents)) this.contents.set(key, value);
		}
		/**
		* Sets a value in this document. For `!!set`, `value` needs to be a
		* boolean to add/remove the item from the set.
		*/
		setIn(path, value) {
			if (Collection.isEmptyPath(path)) this.contents = value;
			else if (this.contents == null) this.contents = Collection.collectionFromPath(this.schema, Array.from(path), value);
			else if (assertCollection(this.contents)) this.contents.setIn(path, value);
		}
		/**
		* Change the YAML version and schema used by the document.
		* A `null` version disables support for directives, explicit tags, anchors, and aliases.
		* It also requires the `schema` option to be given as a `Schema` instance value.
		*
		* Overrides all previously set schema options.
		*/
		setSchema(version, options = {}) {
			if (typeof version === "number") version = String(version);
			let opt;
			switch (version) {
				case "1.1":
					if (this.directives) this.directives.yaml.version = "1.1";
					else this.directives = new directives.Directives({ version: "1.1" });
					opt = {
						resolveKnownTags: false,
						schema: "yaml-1.1"
					};
					break;
				case "1.2":
				case "next":
					if (this.directives) this.directives.yaml.version = version;
					else this.directives = new directives.Directives({ version });
					opt = {
						resolveKnownTags: true,
						schema: "core"
					};
					break;
				case null:
					if (this.directives) delete this.directives;
					opt = null;
					break;
				default: {
					const sv = JSON.stringify(version);
					throw new Error(`Expected '1.1', '1.2' or null as first argument, but found: ${sv}`);
				}
			}
			if (options.schema instanceof Object) this.schema = options.schema;
			else if (opt) this.schema = new Schema.Schema(Object.assign(opt, options));
			else throw new Error(`With a null YAML version, the { schema: Schema } option is required`);
		}
		toJS({ json, jsonArg, mapAsMap, maxAliasCount, onAnchor, reviver } = {}) {
			const ctx = {
				anchors: /* @__PURE__ */ new Map(),
				doc: this,
				keep: !json,
				mapAsMap: mapAsMap === true,
				mapKeyWarned: false,
				maxAliasCount: typeof maxAliasCount === "number" ? maxAliasCount : 100
			};
			const res = toJS.toJS(this.contents, jsonArg ?? "", ctx);
			if (typeof onAnchor === "function") for (const { count, res } of ctx.anchors.values()) onAnchor(res, count);
			return typeof reviver === "function" ? applyReviver.applyReviver(reviver, { "": res }, "", res) : res;
		}
		/**
		* A JSON representation of the document `contents`.
		*
		* @param jsonArg Used by `JSON.stringify` to indicate the array index or
		*   property name.
		*/
		toJSON(jsonArg, onAnchor) {
			return this.toJS({
				json: true,
				jsonArg,
				mapAsMap: false,
				onAnchor
			});
		}
		/** A YAML representation of the document. */
		toString(options = {}) {
			if (this.errors.length > 0) throw new Error("Document with errors cannot be stringified");
			if ("indent" in options && (!Number.isInteger(options.indent) || Number(options.indent) <= 0)) {
				const s = JSON.stringify(options.indent);
				throw new Error(`"indent" option must be a positive integer, not ${s}`);
			}
			return stringifyDocument.stringifyDocument(this, options);
		}
	};
	function assertCollection(contents) {
		if (identity.isCollection(contents)) return true;
		throw new Error("Expected a YAML collection as document contents");
	}
	exports.Document = Document;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/errors.js
var require_errors = /* @__PURE__ */ __commonJSMin(((exports) => {
	var YAMLError = class extends Error {
		constructor(name, pos, code, message) {
			super();
			this.name = name;
			this.code = code;
			this.message = message;
			this.pos = pos;
		}
	};
	var YAMLParseError = class extends YAMLError {
		constructor(pos, code, message) {
			super("YAMLParseError", pos, code, message);
		}
	};
	var YAMLWarning = class extends YAMLError {
		constructor(pos, code, message) {
			super("YAMLWarning", pos, code, message);
		}
	};
	const prettifyError = (src, lc) => (error) => {
		if (error.pos[0] === -1) return;
		error.linePos = error.pos.map((pos) => lc.linePos(pos));
		const { line, col } = error.linePos[0];
		error.message += ` at line ${line}, column ${col}`;
		let ci = col - 1;
		let lineStr = src.substring(lc.lineStarts[line - 1], lc.lineStarts[line]).replace(/[\n\r]+$/, "");
		if (ci >= 60 && lineStr.length > 80) {
			const trimStart = Math.min(ci - 39, lineStr.length - 79);
			lineStr = "…" + lineStr.substring(trimStart);
			ci -= trimStart - 1;
		}
		if (lineStr.length > 80) lineStr = lineStr.substring(0, 79) + "…";
		if (line > 1 && /^ *$/.test(lineStr.substring(0, ci))) {
			let prev = src.substring(lc.lineStarts[line - 2], lc.lineStarts[line - 1]);
			if (prev.length > 80) prev = prev.substring(0, 79) + "…\n";
			lineStr = prev + lineStr;
		}
		if (/[^ ]/.test(lineStr)) {
			let count = 1;
			const end = error.linePos[1];
			if (end?.line === line && end.col > col) count = Math.max(1, Math.min(end.col - col, 80 - ci));
			const pointer = " ".repeat(ci) + "^".repeat(count);
			error.message += `:\n\n${lineStr}\n${pointer}\n`;
		}
	};
	exports.YAMLError = YAMLError;
	exports.YAMLParseError = YAMLParseError;
	exports.YAMLWarning = YAMLWarning;
	exports.prettifyError = prettifyError;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/resolve-props.js
var require_resolve_props = /* @__PURE__ */ __commonJSMin(((exports) => {
	function resolveProps(tokens, { flow, indicator, next, offset, onError, parentIndent, startOnNewline }) {
		let spaceBefore = false;
		let atNewline = startOnNewline;
		let hasSpace = startOnNewline;
		let comment = "";
		let commentSep = "";
		let hasNewline = false;
		let reqSpace = false;
		let tab = null;
		let anchor = null;
		let tag = null;
		let newlineAfterProp = null;
		let comma = null;
		let found = null;
		let start = null;
		for (const token of tokens) {
			if (reqSpace) {
				if (token.type !== "space" && token.type !== "newline" && token.type !== "comma") onError(token.offset, "MISSING_CHAR", "Tags and anchors must be separated from the next token by white space");
				reqSpace = false;
			}
			if (tab) {
				if (atNewline && token.type !== "comment" && token.type !== "newline") onError(tab, "TAB_AS_INDENT", "Tabs are not allowed as indentation");
				tab = null;
			}
			switch (token.type) {
				case "space":
					if (!flow && (indicator !== "doc-start" || next?.type !== "flow-collection") && token.source.includes("	")) tab = token;
					hasSpace = true;
					break;
				case "comment": {
					if (!hasSpace) onError(token, "MISSING_CHAR", "Comments must be separated from other tokens by white space characters");
					const cb = token.source.substring(1) || " ";
					if (!comment) comment = cb;
					else comment += commentSep + cb;
					commentSep = "";
					atNewline = false;
					break;
				}
				case "newline":
					if (atNewline) {
						if (comment) comment += token.source;
						else if (!found || indicator !== "seq-item-ind") spaceBefore = true;
					} else commentSep += token.source;
					atNewline = true;
					hasNewline = true;
					if (anchor || tag) newlineAfterProp = token;
					hasSpace = true;
					break;
				case "anchor":
					if (anchor) onError(token, "MULTIPLE_ANCHORS", "A node can have at most one anchor");
					if (token.source.endsWith(":")) onError(token.offset + token.source.length - 1, "BAD_ALIAS", "Anchor ending in : is ambiguous", true);
					anchor = token;
					start ?? (start = token.offset);
					atNewline = false;
					hasSpace = false;
					reqSpace = true;
					break;
				case "tag":
					if (tag) onError(token, "MULTIPLE_TAGS", "A node can have at most one tag");
					tag = token;
					start ?? (start = token.offset);
					atNewline = false;
					hasSpace = false;
					reqSpace = true;
					break;
				case indicator:
					if (anchor || tag) onError(token, "BAD_PROP_ORDER", `Anchors and tags must be after the ${token.source} indicator`);
					if (found) onError(token, "UNEXPECTED_TOKEN", `Unexpected ${token.source} in ${flow ?? "collection"}`);
					found = token;
					atNewline = indicator === "seq-item-ind" || indicator === "explicit-key-ind";
					hasSpace = false;
					break;
				case "comma": if (flow) {
					if (comma) onError(token, "UNEXPECTED_TOKEN", `Unexpected , in ${flow}`);
					comma = token;
					atNewline = false;
					hasSpace = false;
					break;
				}
				default:
					onError(token, "UNEXPECTED_TOKEN", `Unexpected ${token.type} token`);
					atNewline = false;
					hasSpace = false;
			}
		}
		const last = tokens[tokens.length - 1];
		const end = last ? last.offset + last.source.length : offset;
		if (reqSpace && next && next.type !== "space" && next.type !== "newline" && next.type !== "comma" && (next.type !== "scalar" || next.source !== "")) onError(next.offset, "MISSING_CHAR", "Tags and anchors must be separated from the next token by white space");
		if (tab && (atNewline && tab.indent <= parentIndent || next?.type === "block-map" || next?.type === "block-seq")) onError(tab, "TAB_AS_INDENT", "Tabs are not allowed as indentation");
		return {
			comma,
			found,
			spaceBefore,
			comment,
			hasNewline,
			anchor,
			tag,
			newlineAfterProp,
			end,
			start: start ?? end
		};
	}
	exports.resolveProps = resolveProps;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/util-contains-newline.js
var require_util_contains_newline = /* @__PURE__ */ __commonJSMin(((exports) => {
	function containsNewline(key) {
		if (!key) return null;
		switch (key.type) {
			case "alias":
			case "scalar":
			case "double-quoted-scalar":
			case "single-quoted-scalar":
				if (key.source.includes("\n")) return true;
				if (key.end) {
					for (const st of key.end) if (st.type === "newline") return true;
				}
				return false;
			case "flow-collection":
				for (const it of key.items) {
					for (const st of it.start) if (st.type === "newline") return true;
					if (it.sep) {
						for (const st of it.sep) if (st.type === "newline") return true;
					}
					if (containsNewline(it.key) || containsNewline(it.value)) return true;
				}
				return false;
			default: return true;
		}
	}
	exports.containsNewline = containsNewline;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/util-flow-indent-check.js
var require_util_flow_indent_check = /* @__PURE__ */ __commonJSMin(((exports) => {
	var utilContainsNewline = require_util_contains_newline();
	function flowIndentCheck(indent, fc, onError) {
		if (fc?.type === "flow-collection") {
			const end = fc.end[0];
			if (end.indent === indent && (end.source === "]" || end.source === "}") && utilContainsNewline.containsNewline(fc)) onError(end, "BAD_INDENT", "Flow end indicator should be more indented than parent", true);
		}
	}
	exports.flowIndentCheck = flowIndentCheck;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/util-map-includes.js
var require_util_map_includes = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	function mapIncludes(ctx, items, search) {
		const { uniqueKeys } = ctx.options;
		if (uniqueKeys === false) return false;
		const isEqual = typeof uniqueKeys === "function" ? uniqueKeys : (a, b) => a === b || identity.isScalar(a) && identity.isScalar(b) && a.value === b.value;
		return items.some((pair) => isEqual(pair.key, search));
	}
	exports.mapIncludes = mapIncludes;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/resolve-block-map.js
var require_resolve_block_map = /* @__PURE__ */ __commonJSMin(((exports) => {
	var Pair = require_Pair();
	var YAMLMap = require_YAMLMap();
	var resolveProps = require_resolve_props();
	var utilContainsNewline = require_util_contains_newline();
	var utilFlowIndentCheck = require_util_flow_indent_check();
	var utilMapIncludes = require_util_map_includes();
	const startColMsg = "All mapping items must start at the same column";
	function resolveBlockMap({ composeNode, composeEmptyNode }, ctx, bm, onError, tag) {
		const map = new ((tag?.nodeClass) ?? YAMLMap.YAMLMap)(ctx.schema);
		if (ctx.atRoot) ctx.atRoot = false;
		let offset = bm.offset;
		let commentEnd = null;
		for (const collItem of bm.items) {
			const { start, key, sep, value } = collItem;
			const keyProps = resolveProps.resolveProps(start, {
				indicator: "explicit-key-ind",
				next: key ?? sep?.[0],
				offset,
				onError,
				parentIndent: bm.indent,
				startOnNewline: true
			});
			const implicitKey = !keyProps.found;
			if (implicitKey) {
				if (key) {
					if (key.type === "block-seq") onError(offset, "BLOCK_AS_IMPLICIT_KEY", "A block sequence may not be used as an implicit map key");
					else if ("indent" in key && key.indent !== bm.indent) onError(offset, "BAD_INDENT", startColMsg);
				}
				if (!keyProps.anchor && !keyProps.tag && !sep) {
					commentEnd = keyProps.end;
					if (keyProps.comment) {
						if (map.comment) map.comment += "\n" + keyProps.comment;
						else map.comment = keyProps.comment;
					}
					continue;
				}
				if (keyProps.newlineAfterProp || utilContainsNewline.containsNewline(key)) onError(key ?? start[start.length - 1], "MULTILINE_IMPLICIT_KEY", "Implicit keys need to be on a single line");
			} else if (keyProps.found?.indent !== bm.indent) onError(offset, "BAD_INDENT", startColMsg);
			ctx.atKey = true;
			const keyStart = keyProps.end;
			const keyNode = key ? composeNode(ctx, key, keyProps, onError) : composeEmptyNode(ctx, keyStart, start, null, keyProps, onError);
			if (ctx.schema.compat) utilFlowIndentCheck.flowIndentCheck(bm.indent, key, onError);
			ctx.atKey = false;
			if (utilMapIncludes.mapIncludes(ctx, map.items, keyNode)) onError(keyStart, "DUPLICATE_KEY", "Map keys must be unique");
			const valueProps = resolveProps.resolveProps(sep ?? [], {
				indicator: "map-value-ind",
				next: value,
				offset: keyNode.range[2],
				onError,
				parentIndent: bm.indent,
				startOnNewline: !key || key.type === "block-scalar"
			});
			offset = valueProps.end;
			if (valueProps.found) {
				if (implicitKey) {
					if (value?.type === "block-map" && !valueProps.hasNewline) onError(offset, "BLOCK_AS_IMPLICIT_KEY", "Nested mappings are not allowed in compact mappings");
					if (ctx.options.strict && keyProps.start < valueProps.found.offset - 1024) onError(keyNode.range, "KEY_OVER_1024_CHARS", "The : indicator must be at most 1024 chars after the start of an implicit block mapping key");
				}
				const valueNode = value ? composeNode(ctx, value, valueProps, onError) : composeEmptyNode(ctx, offset, sep, null, valueProps, onError);
				if (ctx.schema.compat) utilFlowIndentCheck.flowIndentCheck(bm.indent, value, onError);
				offset = valueNode.range[2];
				const pair = new Pair.Pair(keyNode, valueNode);
				if (ctx.options.keepSourceTokens) pair.srcToken = collItem;
				map.items.push(pair);
			} else {
				if (implicitKey) onError(keyNode.range, "MISSING_CHAR", "Implicit map keys need to be followed by map values");
				if (valueProps.comment) {
					if (keyNode.comment) keyNode.comment += "\n" + valueProps.comment;
					else keyNode.comment = valueProps.comment;
				}
				const pair = new Pair.Pair(keyNode);
				if (ctx.options.keepSourceTokens) pair.srcToken = collItem;
				map.items.push(pair);
			}
		}
		if (commentEnd && commentEnd < offset) onError(commentEnd, "IMPOSSIBLE", "Map comment with trailing content");
		map.range = [
			bm.offset,
			offset,
			commentEnd ?? offset
		];
		return map;
	}
	exports.resolveBlockMap = resolveBlockMap;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/resolve-block-seq.js
var require_resolve_block_seq = /* @__PURE__ */ __commonJSMin(((exports) => {
	var YAMLSeq = require_YAMLSeq();
	var resolveProps = require_resolve_props();
	var utilFlowIndentCheck = require_util_flow_indent_check();
	function resolveBlockSeq({ composeNode, composeEmptyNode }, ctx, bs, onError, tag) {
		const seq = new ((tag?.nodeClass) ?? YAMLSeq.YAMLSeq)(ctx.schema);
		if (ctx.atRoot) ctx.atRoot = false;
		if (ctx.atKey) ctx.atKey = false;
		let offset = bs.offset;
		let commentEnd = null;
		for (const { start, value } of bs.items) {
			const props = resolveProps.resolveProps(start, {
				indicator: "seq-item-ind",
				next: value,
				offset,
				onError,
				parentIndent: bs.indent,
				startOnNewline: true
			});
			if (!props.found) {
				if (props.anchor || props.tag || value) {
					if (value?.type === "block-seq") onError(props.end, "BAD_INDENT", "All sequence items must start at the same column");
					else onError(offset, "MISSING_CHAR", "Sequence item without - indicator");
				} else {
					commentEnd = props.end;
					if (props.comment) seq.comment = props.comment;
					continue;
				}
			}
			const node = value ? composeNode(ctx, value, props, onError) : composeEmptyNode(ctx, props.end, start, null, props, onError);
			if (ctx.schema.compat) utilFlowIndentCheck.flowIndentCheck(bs.indent, value, onError);
			offset = node.range[2];
			seq.items.push(node);
		}
		seq.range = [
			bs.offset,
			offset,
			commentEnd ?? offset
		];
		return seq;
	}
	exports.resolveBlockSeq = resolveBlockSeq;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/resolve-end.js
var require_resolve_end = /* @__PURE__ */ __commonJSMin(((exports) => {
	function resolveEnd(end, offset, reqSpace, onError) {
		let comment = "";
		if (end) {
			let hasSpace = false;
			let sep = "";
			for (const token of end) {
				const { source, type } = token;
				switch (type) {
					case "space":
						hasSpace = true;
						break;
					case "comment": {
						if (reqSpace && !hasSpace) onError(token, "MISSING_CHAR", "Comments must be separated from other tokens by white space characters");
						const cb = source.substring(1) || " ";
						if (!comment) comment = cb;
						else comment += sep + cb;
						sep = "";
						break;
					}
					case "newline":
						if (comment) sep += source;
						hasSpace = true;
						break;
					default: onError(token, "UNEXPECTED_TOKEN", `Unexpected ${type} at node end`);
				}
				offset += source.length;
			}
		}
		return {
			comment,
			offset
		};
	}
	exports.resolveEnd = resolveEnd;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/resolve-flow-collection.js
var require_resolve_flow_collection = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var Pair = require_Pair();
	var YAMLMap = require_YAMLMap();
	var YAMLSeq = require_YAMLSeq();
	var resolveEnd = require_resolve_end();
	var resolveProps = require_resolve_props();
	var utilContainsNewline = require_util_contains_newline();
	var utilMapIncludes = require_util_map_includes();
	const blockMsg = "Block collections are not allowed within flow collections";
	const isBlock = (token) => token && (token.type === "block-map" || token.type === "block-seq");
	function resolveFlowCollection({ composeNode, composeEmptyNode }, ctx, fc, onError, tag) {
		const isMap = fc.start.source === "{";
		const fcName = isMap ? "flow map" : "flow sequence";
		const coll = new ((tag?.nodeClass) ?? (isMap ? YAMLMap.YAMLMap : YAMLSeq.YAMLSeq))(ctx.schema);
		coll.flow = true;
		const atRoot = ctx.atRoot;
		if (atRoot) ctx.atRoot = false;
		if (ctx.atKey) ctx.atKey = false;
		let offset = fc.offset + fc.start.source.length;
		for (let i = 0; i < fc.items.length; ++i) {
			const collItem = fc.items[i];
			const { start, key, sep, value } = collItem;
			const props = resolveProps.resolveProps(start, {
				flow: fcName,
				indicator: "explicit-key-ind",
				next: key ?? sep?.[0],
				offset,
				onError,
				parentIndent: fc.indent,
				startOnNewline: false
			});
			if (!props.found) {
				if (!props.anchor && !props.tag && !sep && !value) {
					if (i === 0 && props.comma) onError(props.comma, "UNEXPECTED_TOKEN", `Unexpected , in ${fcName}`);
					else if (i < fc.items.length - 1) onError(props.start, "UNEXPECTED_TOKEN", `Unexpected empty item in ${fcName}`);
					if (props.comment) {
						if (coll.comment) coll.comment += "\n" + props.comment;
						else coll.comment = props.comment;
					}
					offset = props.end;
					continue;
				}
				if (!isMap && ctx.options.strict && utilContainsNewline.containsNewline(key)) onError(key, "MULTILINE_IMPLICIT_KEY", "Implicit keys of flow sequence pairs need to be on a single line");
			}
			if (i === 0) {
				if (props.comma) onError(props.comma, "UNEXPECTED_TOKEN", `Unexpected , in ${fcName}`);
			} else {
				if (!props.comma) onError(props.start, "MISSING_CHAR", `Missing , between ${fcName} items`);
				if (props.comment) {
					let prevItemComment = "";
					loop: for (const st of start) switch (st.type) {
						case "comma":
						case "space": break;
						case "comment":
							prevItemComment = st.source.substring(1);
							break loop;
						default: break loop;
					}
					if (prevItemComment) {
						let prev = coll.items[coll.items.length - 1];
						if (identity.isPair(prev)) prev = prev.value ?? prev.key;
						if (prev.comment) prev.comment += "\n" + prevItemComment;
						else prev.comment = prevItemComment;
						props.comment = props.comment.substring(prevItemComment.length + 1);
					}
				}
			}
			if (!isMap && !sep && !props.found) {
				const valueNode = value ? composeNode(ctx, value, props, onError) : composeEmptyNode(ctx, props.end, sep, null, props, onError);
				coll.items.push(valueNode);
				offset = valueNode.range[2];
				if (isBlock(value)) onError(valueNode.range, "BLOCK_IN_FLOW", blockMsg);
			} else {
				ctx.atKey = true;
				const keyStart = props.end;
				const keyNode = key ? composeNode(ctx, key, props, onError) : composeEmptyNode(ctx, keyStart, start, null, props, onError);
				if (isBlock(key)) onError(keyNode.range, "BLOCK_IN_FLOW", blockMsg);
				ctx.atKey = false;
				const valueProps = resolveProps.resolveProps(sep ?? [], {
					flow: fcName,
					indicator: "map-value-ind",
					next: value,
					offset: keyNode.range[2],
					onError,
					parentIndent: fc.indent,
					startOnNewline: false
				});
				if (valueProps.found) {
					if (!isMap && !props.found && ctx.options.strict) {
						if (sep) for (const st of sep) {
							if (st === valueProps.found) break;
							if (st.type === "newline") {
								onError(st, "MULTILINE_IMPLICIT_KEY", "Implicit keys of flow sequence pairs need to be on a single line");
								break;
							}
						}
						if (props.start < valueProps.found.offset - 1024) onError(valueProps.found, "KEY_OVER_1024_CHARS", "The : indicator must be at most 1024 chars after the start of an implicit flow sequence key");
					}
				} else if (value) {
					if ("source" in value && value.source?.[0] === ":") onError(value, "MISSING_CHAR", `Missing space after : in ${fcName}`);
					else onError(valueProps.start, "MISSING_CHAR", `Missing , or : between ${fcName} items`);
				}
				const valueNode = value ? composeNode(ctx, value, valueProps, onError) : valueProps.found ? composeEmptyNode(ctx, valueProps.end, sep, null, valueProps, onError) : null;
				if (valueNode) {
					if (isBlock(value)) onError(valueNode.range, "BLOCK_IN_FLOW", blockMsg);
				} else if (valueProps.comment) {
					if (keyNode.comment) keyNode.comment += "\n" + valueProps.comment;
					else keyNode.comment = valueProps.comment;
				}
				const pair = new Pair.Pair(keyNode, valueNode);
				if (ctx.options.keepSourceTokens) pair.srcToken = collItem;
				if (isMap) {
					const map = coll;
					if (utilMapIncludes.mapIncludes(ctx, map.items, keyNode)) onError(keyStart, "DUPLICATE_KEY", "Map keys must be unique");
					map.items.push(pair);
				} else {
					const map = new YAMLMap.YAMLMap(ctx.schema);
					map.flow = true;
					map.items.push(pair);
					const endRange = (valueNode ?? keyNode).range;
					map.range = [
						keyNode.range[0],
						endRange[1],
						endRange[2]
					];
					coll.items.push(map);
				}
				offset = valueNode ? valueNode.range[2] : valueProps.end;
			}
		}
		const expectedEnd = isMap ? "}" : "]";
		const [ce, ...ee] = fc.end;
		let cePos = offset;
		if (ce?.source === expectedEnd) cePos = ce.offset + ce.source.length;
		else {
			const name = fcName[0].toUpperCase() + fcName.substring(1);
			const msg = atRoot ? `${name} must end with a ${expectedEnd}` : `${name} in block collection must be sufficiently indented and end with a ${expectedEnd}`;
			onError(offset, atRoot ? "MISSING_CHAR" : "BAD_INDENT", msg);
			if (ce && ce.source.length !== 1) ee.unshift(ce);
		}
		if (ee.length > 0) {
			const end = resolveEnd.resolveEnd(ee, cePos, ctx.options.strict, onError);
			if (end.comment) {
				if (coll.comment) coll.comment += "\n" + end.comment;
				else coll.comment = end.comment;
			}
			coll.range = [
				fc.offset,
				cePos,
				end.offset
			];
		} else coll.range = [
			fc.offset,
			cePos,
			cePos
		];
		return coll;
	}
	exports.resolveFlowCollection = resolveFlowCollection;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/compose-collection.js
var require_compose_collection = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var Scalar = require_Scalar();
	var YAMLMap = require_YAMLMap();
	var YAMLSeq = require_YAMLSeq();
	var resolveBlockMap = require_resolve_block_map();
	var resolveBlockSeq = require_resolve_block_seq();
	var resolveFlowCollection = require_resolve_flow_collection();
	function resolveCollection(CN, ctx, token, onError, tagName, tag) {
		const coll = token.type === "block-map" ? resolveBlockMap.resolveBlockMap(CN, ctx, token, onError, tag) : token.type === "block-seq" ? resolveBlockSeq.resolveBlockSeq(CN, ctx, token, onError, tag) : resolveFlowCollection.resolveFlowCollection(CN, ctx, token, onError, tag);
		const Coll = coll.constructor;
		if (tagName === "!" || tagName === Coll.tagName) {
			coll.tag = Coll.tagName;
			return coll;
		}
		if (tagName) coll.tag = tagName;
		return coll;
	}
	function composeCollection(CN, ctx, token, props, onError) {
		const tagToken = props.tag;
		const tagName = !tagToken ? null : ctx.directives.tagName(tagToken.source, (msg) => onError(tagToken, "TAG_RESOLVE_FAILED", msg));
		if (token.type === "block-seq") {
			const { anchor, newlineAfterProp: nl } = props;
			const lastProp = anchor && tagToken ? anchor.offset > tagToken.offset ? anchor : tagToken : anchor ?? tagToken;
			if (lastProp && (!nl || nl.offset < lastProp.offset)) onError(lastProp, "MISSING_CHAR", "Missing newline after block sequence props");
		}
		const expType = token.type === "block-map" ? "map" : token.type === "block-seq" ? "seq" : token.start.source === "{" ? "map" : "seq";
		if (!tagToken || !tagName || tagName === "!" || tagName === YAMLMap.YAMLMap.tagName && expType === "map" || tagName === YAMLSeq.YAMLSeq.tagName && expType === "seq") return resolveCollection(CN, ctx, token, onError, tagName);
		let tag = ctx.schema.tags.find((t) => t.tag === tagName && t.collection === expType);
		if (!tag) {
			const kt = ctx.schema.knownTags[tagName];
			if (kt?.collection === expType) {
				ctx.schema.tags.push(Object.assign({}, kt, { default: false }));
				tag = kt;
			} else {
				if (kt) onError(tagToken, "BAD_COLLECTION_TYPE", `${kt.tag} used for ${expType} collection, but expects ${kt.collection ?? "scalar"}`, true);
				else onError(tagToken, "TAG_RESOLVE_FAILED", `Unresolved tag: ${tagName}`, true);
				return resolveCollection(CN, ctx, token, onError, tagName);
			}
		}
		const coll = resolveCollection(CN, ctx, token, onError, tagName, tag);
		const res = tag.resolve?.(coll, (msg) => onError(tagToken, "TAG_RESOLVE_FAILED", msg), ctx.options) ?? coll;
		const node = identity.isNode(res) ? res : new Scalar.Scalar(res);
		node.range = coll.range;
		node.tag = tagName;
		if (tag?.format) node.format = tag.format;
		return node;
	}
	exports.composeCollection = composeCollection;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/resolve-block-scalar.js
var require_resolve_block_scalar = /* @__PURE__ */ __commonJSMin(((exports) => {
	var Scalar = require_Scalar();
	function resolveBlockScalar(ctx, scalar, onError) {
		const start = scalar.offset;
		const header = parseBlockScalarHeader(scalar, ctx.options.strict, onError);
		if (!header) return {
			value: "",
			type: null,
			comment: "",
			range: [
				start,
				start,
				start
			]
		};
		const type = header.mode === ">" ? Scalar.Scalar.BLOCK_FOLDED : Scalar.Scalar.BLOCK_LITERAL;
		const lines = scalar.source ? splitLines(scalar.source) : [];
		let chompStart = lines.length;
		for (let i = lines.length - 1; i >= 0; --i) {
			const content = lines[i][1];
			if (content === "" || content === "\r") chompStart = i;
			else break;
		}
		if (chompStart === 0) {
			const value = header.chomp === "+" && lines.length > 0 ? "\n".repeat(Math.max(1, lines.length - 1)) : "";
			let end = start + header.length;
			if (scalar.source) end += scalar.source.length;
			return {
				value,
				type,
				comment: header.comment,
				range: [
					start,
					end,
					end
				]
			};
		}
		let trimIndent = scalar.indent + header.indent;
		let offset = scalar.offset + header.length;
		let contentStart = 0;
		for (let i = 0; i < chompStart; ++i) {
			const [indent, content] = lines[i];
			if (content === "" || content === "\r") {
				if (header.indent === 0 && indent.length > trimIndent) trimIndent = indent.length;
			} else {
				if (indent.length < trimIndent) onError(offset + indent.length, "MISSING_CHAR", "Block scalars with more-indented leading empty lines must use an explicit indentation indicator");
				if (header.indent === 0) trimIndent = indent.length;
				contentStart = i;
				if (trimIndent === 0 && !ctx.atRoot) onError(offset, "BAD_INDENT", "Block scalar values in collections must be indented");
				break;
			}
			offset += indent.length + content.length + 1;
		}
		for (let i = lines.length - 1; i >= chompStart; --i) if (lines[i][0].length > trimIndent) chompStart = i + 1;
		let value = "";
		let sep = "";
		let prevMoreIndented = false;
		for (let i = 0; i < contentStart; ++i) value += lines[i][0].slice(trimIndent) + "\n";
		for (let i = contentStart; i < chompStart; ++i) {
			let [indent, content] = lines[i];
			offset += indent.length + content.length + 1;
			const crlf = content[content.length - 1] === "\r";
			if (crlf) content = content.slice(0, -1);
			/* istanbul ignore if already caught in lexer */
			if (content && indent.length < trimIndent) {
				const message = `Block scalar lines must not be less indented than their ${header.indent ? "explicit indentation indicator" : "first line"}`;
				onError(offset - content.length - (crlf ? 2 : 1), "BAD_INDENT", message);
				indent = "";
			}
			if (type === Scalar.Scalar.BLOCK_LITERAL) {
				value += sep + indent.slice(trimIndent) + content;
				sep = "\n";
			} else if (indent.length > trimIndent || content[0] === "	") {
				if (sep === " ") sep = "\n";
				else if (!prevMoreIndented && sep === "\n") sep = "\n\n";
				value += sep + indent.slice(trimIndent) + content;
				sep = "\n";
				prevMoreIndented = true;
			} else if (content === "") {
				if (sep === "\n") value += "\n";
				else sep = "\n";
			} else {
				value += sep + content;
				sep = " ";
				prevMoreIndented = false;
			}
		}
		switch (header.chomp) {
			case "-": break;
			case "+":
				for (let i = chompStart; i < lines.length; ++i) value += "\n" + lines[i][0].slice(trimIndent);
				if (value[value.length - 1] !== "\n") value += "\n";
				break;
			default: value += "\n";
		}
		const end = start + header.length + scalar.source.length;
		return {
			value,
			type,
			comment: header.comment,
			range: [
				start,
				end,
				end
			]
		};
	}
	function parseBlockScalarHeader({ offset, props }, strict, onError) {
		/* istanbul ignore if should not happen */
		if (props[0].type !== "block-scalar-header") {
			onError(props[0], "IMPOSSIBLE", "Block scalar header not found");
			return null;
		}
		const { source } = props[0];
		const mode = source[0];
		let indent = 0;
		let chomp = "";
		let error = -1;
		for (let i = 1; i < source.length; ++i) {
			const ch = source[i];
			if (!chomp && (ch === "-" || ch === "+")) chomp = ch;
			else {
				const n = Number(ch);
				if (!indent && n) indent = n;
				else if (error === -1) error = offset + i;
			}
		}
		if (error !== -1) onError(error, "UNEXPECTED_TOKEN", `Block scalar header includes extra characters: ${source}`);
		let hasSpace = false;
		let comment = "";
		let length = source.length;
		for (let i = 1; i < props.length; ++i) {
			const token = props[i];
			switch (token.type) {
				case "space": hasSpace = true;
				case "newline":
					length += token.source.length;
					break;
				case "comment":
					if (strict && !hasSpace) onError(token, "MISSING_CHAR", "Comments must be separated from other tokens by white space characters");
					length += token.source.length;
					comment = token.source.substring(1);
					break;
				case "error":
					onError(token, "UNEXPECTED_TOKEN", token.message);
					length += token.source.length;
					break;
				/* istanbul ignore next should not happen */
				default: {
					onError(token, "UNEXPECTED_TOKEN", `Unexpected token in block scalar header: ${token.type}`);
					const ts = token.source;
					if (ts && typeof ts === "string") length += ts.length;
				}
			}
		}
		return {
			mode,
			indent,
			chomp,
			comment,
			length
		};
	}
	/** @returns Array of lines split up as `[indent, content]` */
	function splitLines(source) {
		const split = source.split(/\n( *)/);
		const first = split[0];
		const m = first.match(/^( *)/);
		const lines = [m?.[1] ? [m[1], first.slice(m[1].length)] : ["", first]];
		for (let i = 1; i < split.length; i += 2) lines.push([split[i], split[i + 1]]);
		return lines;
	}
	exports.resolveBlockScalar = resolveBlockScalar;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/resolve-flow-scalar.js
var require_resolve_flow_scalar = /* @__PURE__ */ __commonJSMin(((exports) => {
	var Scalar = require_Scalar();
	var resolveEnd = require_resolve_end();
	function resolveFlowScalar(scalar, strict, onError) {
		const { offset, type, source, end } = scalar;
		let _type;
		let value;
		const _onError = (rel, code, msg) => onError(offset + rel, code, msg);
		switch (type) {
			case "scalar":
				_type = Scalar.Scalar.PLAIN;
				value = plainValue(source, _onError);
				break;
			case "single-quoted-scalar":
				_type = Scalar.Scalar.QUOTE_SINGLE;
				value = singleQuotedValue(source, _onError);
				break;
			case "double-quoted-scalar":
				_type = Scalar.Scalar.QUOTE_DOUBLE;
				value = doubleQuotedValue(source, _onError);
				break;
			/* istanbul ignore next should not happen */
			default:
				onError(scalar, "UNEXPECTED_TOKEN", `Expected a flow scalar value, but found: ${type}`);
				return {
					value: "",
					type: null,
					comment: "",
					range: [
						offset,
						offset + source.length,
						offset + source.length
					]
				};
		}
		const valueEnd = offset + source.length;
		const re = resolveEnd.resolveEnd(end, valueEnd, strict, onError);
		return {
			value,
			type: _type,
			comment: re.comment,
			range: [
				offset,
				valueEnd,
				re.offset
			]
		};
	}
	function plainValue(source, onError) {
		let badChar = "";
		switch (source[0]) {
			/* istanbul ignore next should not happen */
			case "	":
				badChar = "a tab character";
				break;
			case ",":
				badChar = "flow indicator character ,";
				break;
			case "%":
				badChar = "directive indicator character %";
				break;
			case "|":
			case ">":
				badChar = `block scalar indicator ${source[0]}`;
				break;
			case "@":
			case "`": badChar = `reserved character ${source[0]}`;
		}
		if (badChar) onError(0, "BAD_SCALAR_START", `Plain value cannot start with ${badChar}`);
		return unfoldLines(source);
	}
	function singleQuotedValue(source, onError) {
		if (source[source.length - 1] !== "'" || source.length === 1) onError(source.length, "MISSING_CHAR", "Missing closing 'quote");
		return unfoldLines(source.slice(1, -1)).replace(/''/g, "'");
	}
	function unfoldLines(source) {
		const line = /(.*?)\r?\n/sy;
		let match = line.exec(source);
		if (!match) return source;
		/**
		* The negative lookbehinds in these RegExps are to
		* prevent causing a polynomial search time in certain cases.
		*
		* The try-catch is for Safari < 16.4 and other old browsers:
		* https://caniuse.com/js-regexp-lookbehind
		*/
		let trimEnd, trimBoth;
		try {
			trimEnd = new RegExp("(?<![ 	])[ 	]+$");
			trimBoth = new RegExp("^[ 	]+|(?<![ 	])[ 	]+$", "g");
		} catch {
			trimEnd = /[ \t]+$/;
			trimBoth = /^[ \t]+|[ \t]+$/g;
		}
		let res = match[1].replace(trimEnd, "");
		let sep = " ";
		let pos = line.lastIndex;
		while (match = line.exec(source)) {
			const lm = match[1].replace(trimBoth, "");
			if (lm === "") {
				if (sep === "\n") res += sep;
				else sep = "\n";
			} else {
				res += sep + lm;
				sep = " ";
			}
			pos = line.lastIndex;
		}
		const last = /[ \t]*(.*)/sy;
		last.lastIndex = pos;
		match = last.exec(source);
		return res + sep + (match?.[1] ?? "");
	}
	function doubleQuotedValue(source, onError) {
		let res = "";
		for (let i = 1; i < source.length - 1; ++i) {
			const ch = source[i];
			if (ch === "\r" && source[i + 1] === "\n") continue;
			if (ch === "\n") {
				const { fold, offset } = foldNewline(source, i);
				res += fold;
				i = offset;
			} else if (ch === "\\") {
				let next = source[++i];
				const cc = escapeCodes[next];
				if (cc) res += cc;
				else if (next === "\n") {
					next = source[i + 1];
					while (next === " " || next === "	") next = source[++i + 1];
				} else if (next === "\r" && source[i + 1] === "\n") {
					next = source[++i + 1];
					while (next === " " || next === "	") next = source[++i + 1];
				} else if (next === "x" || next === "u" || next === "U") {
					const length = next === "x" ? 2 : next === "u" ? 4 : 8;
					res += parseCharCode(source, i + 1, length, onError);
					i += length;
				} else {
					const raw = source.substr(i - 1, 2);
					onError(i - 1, "BAD_DQ_ESCAPE", `Invalid escape sequence ${raw}`);
					res += raw;
				}
			} else if (ch === " " || ch === "	") {
				const wsStart = i;
				let next = source[i + 1];
				while (next === " " || next === "	") next = source[++i + 1];
				if (next !== "\n" && !(next === "\r" && source[i + 2] === "\n")) res += i > wsStart ? source.slice(wsStart, i + 1) : ch;
			} else res += ch;
		}
		if (source[source.length - 1] !== "\"" || source.length === 1) onError(source.length, "MISSING_CHAR", "Missing closing \"quote");
		return res;
	}
	/**
	* Fold a single newline into a space, multiple newlines to N - 1 newlines.
	* Presumes `source[offset] === '\n'`
	*/
	function foldNewline(source, offset) {
		let fold = "";
		let ch = source[offset + 1];
		while (ch === " " || ch === "	" || ch === "\n" || ch === "\r") {
			if (ch === "\r" && source[offset + 2] !== "\n") break;
			if (ch === "\n") fold += "\n";
			offset += 1;
			ch = source[offset + 1];
		}
		if (!fold) fold = " ";
		return {
			fold,
			offset
		};
	}
	const escapeCodes = {
		"0": "\0",
		a: "\x07",
		b: "\b",
		e: "\x1B",
		f: "\f",
		n: "\n",
		r: "\r",
		t: "	",
		v: "\v",
		N: "",
		_: "\xA0",
		L: "\u2028",
		P: "\u2029",
		" ": " ",
		"\"": "\"",
		"/": "/",
		"\\": "\\",
		"	": "	"
	};
	function parseCharCode(source, offset, length, onError) {
		const cc = source.substr(offset, length);
		const code = cc.length === length && /^[0-9a-fA-F]+$/.test(cc) ? parseInt(cc, 16) : NaN;
		try {
			return String.fromCodePoint(code);
		} catch {
			const raw = source.substr(offset - 2, length + 2);
			onError(offset - 2, "BAD_DQ_ESCAPE", `Invalid escape sequence ${raw}`);
			return raw;
		}
	}
	exports.resolveFlowScalar = resolveFlowScalar;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/compose-scalar.js
var require_compose_scalar = /* @__PURE__ */ __commonJSMin(((exports) => {
	var identity = require_identity();
	var Scalar = require_Scalar();
	var resolveBlockScalar = require_resolve_block_scalar();
	var resolveFlowScalar = require_resolve_flow_scalar();
	function composeScalar(ctx, token, tagToken, onError) {
		const { value, type, comment, range } = token.type === "block-scalar" ? resolveBlockScalar.resolveBlockScalar(ctx, token, onError) : resolveFlowScalar.resolveFlowScalar(token, ctx.options.strict, onError);
		const tagName = tagToken ? ctx.directives.tagName(tagToken.source, (msg) => onError(tagToken, "TAG_RESOLVE_FAILED", msg)) : null;
		let tag;
		if (ctx.options.stringKeys && ctx.atKey) tag = ctx.schema[identity.SCALAR];
		else if (tagName) tag = findScalarTagByName(ctx.schema, value, tagName, tagToken, onError);
		else if (token.type === "scalar") tag = findScalarTagByTest(ctx, value, token, onError);
		else tag = ctx.schema[identity.SCALAR];
		let scalar;
		try {
			const res = tag.resolve(value, (msg) => onError(tagToken ?? token, "TAG_RESOLVE_FAILED", msg), ctx.options);
			scalar = identity.isScalar(res) ? res : new Scalar.Scalar(res);
		} catch (error) {
			const msg = error instanceof Error ? error.message : String(error);
			onError(tagToken ?? token, "TAG_RESOLVE_FAILED", msg);
			scalar = new Scalar.Scalar(value);
		}
		scalar.range = range;
		scalar.source = value;
		if (type) scalar.type = type;
		if (tagName) scalar.tag = tagName;
		if (tag.format) scalar.format = tag.format;
		if (comment) scalar.comment = comment;
		return scalar;
	}
	function findScalarTagByName(schema, value, tagName, tagToken, onError) {
		if (tagName === "!") return schema[identity.SCALAR];
		const matchWithTest = [];
		for (const tag of schema.tags) if (!tag.collection && tag.tag === tagName) {
			if (tag.default && tag.test) matchWithTest.push(tag);
			else return tag;
		}
		for (const tag of matchWithTest) if (tag.test?.test(value)) return tag;
		const kt = schema.knownTags[tagName];
		if (kt && !kt.collection) {
			schema.tags.push(Object.assign({}, kt, {
				default: false,
				test: void 0
			}));
			return kt;
		}
		onError(tagToken, "TAG_RESOLVE_FAILED", `Unresolved tag: ${tagName}`, tagName !== "tag:yaml.org,2002:str");
		return schema[identity.SCALAR];
	}
	function findScalarTagByTest({ atKey, directives, schema }, value, token, onError) {
		const tag = schema.tags.find((tag) => (tag.default === true || atKey && tag.default === "key") && tag.test?.test(value)) || schema[identity.SCALAR];
		if (schema.compat) {
			const compat = schema.compat.find((tag) => tag.default && tag.test?.test(value)) ?? schema[identity.SCALAR];
			if (tag.tag !== compat.tag) onError(token, "TAG_RESOLVE_FAILED", `Value may be parsed as either ${directives.tagString(tag.tag)} or ${directives.tagString(compat.tag)}`, true);
		}
		return tag;
	}
	exports.composeScalar = composeScalar;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/util-empty-scalar-position.js
var require_util_empty_scalar_position = /* @__PURE__ */ __commonJSMin(((exports) => {
	function emptyScalarPosition(offset, before, pos) {
		if (before) {
			pos ?? (pos = before.length);
			for (let i = pos - 1; i >= 0; --i) {
				let st = before[i];
				switch (st.type) {
					case "space":
					case "comment":
					case "newline":
						offset -= st.source.length;
						continue;
				}
				st = before[++i];
				while (st?.type === "space") {
					offset += st.source.length;
					st = before[++i];
				}
				break;
			}
		}
		return offset;
	}
	exports.emptyScalarPosition = emptyScalarPosition;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/compose-node.js
var require_compose_node = /* @__PURE__ */ __commonJSMin(((exports) => {
	var Alias = require_Alias();
	var identity = require_identity();
	var composeCollection = require_compose_collection();
	var composeScalar = require_compose_scalar();
	var resolveEnd = require_resolve_end();
	var utilEmptyScalarPosition = require_util_empty_scalar_position();
	const CN = {
		composeNode,
		composeEmptyNode
	};
	function composeNode(ctx, token, props, onError) {
		const atKey = ctx.atKey;
		const { spaceBefore, comment, anchor, tag } = props;
		let node;
		let isSrcToken = true;
		switch (token.type) {
			case "alias":
				node = composeAlias(ctx, token, onError);
				if (anchor || tag) onError(token, "ALIAS_PROPS", "An alias node must not specify any properties");
				break;
			case "scalar":
			case "single-quoted-scalar":
			case "double-quoted-scalar":
			case "block-scalar":
				node = composeScalar.composeScalar(ctx, token, tag, onError);
				if (anchor) node.anchor = anchor.source.substring(1);
				break;
			case "block-map":
			case "block-seq":
			case "flow-collection":
				try {
					node = composeCollection.composeCollection(CN, ctx, token, props, onError);
					if (anchor) node.anchor = anchor.source.substring(1);
				} catch (error) {
					onError(token, "RESOURCE_EXHAUSTION", error instanceof Error ? error.message : String(error));
				}
				break;
			default:
				onError(token, "UNEXPECTED_TOKEN", token.type === "error" ? token.message : `Unsupported token (type: ${token.type})`);
				isSrcToken = false;
		}
		node ?? (node = composeEmptyNode(ctx, token.offset, void 0, null, props, onError));
		if (anchor && node.anchor === "") onError(anchor, "BAD_ALIAS", "Anchor cannot be an empty string");
		if (atKey && ctx.options.stringKeys && (!identity.isScalar(node) || typeof node.value !== "string" || node.tag && node.tag !== "tag:yaml.org,2002:str")) onError(tag ?? token, "NON_STRING_KEY", "With stringKeys, all keys must be strings");
		if (spaceBefore) node.spaceBefore = true;
		if (comment) {
			if (token.type === "scalar" && token.source === "") node.comment = comment;
			else node.commentBefore = comment;
		}
		if (ctx.options.keepSourceTokens && isSrcToken) node.srcToken = token;
		return node;
	}
	function composeEmptyNode(ctx, offset, before, pos, { spaceBefore, comment, anchor, tag, end }, onError) {
		const token = {
			type: "scalar",
			offset: utilEmptyScalarPosition.emptyScalarPosition(offset, before, pos),
			indent: -1,
			source: ""
		};
		const node = composeScalar.composeScalar(ctx, token, tag, onError);
		if (anchor) {
			node.anchor = anchor.source.substring(1);
			if (node.anchor === "") onError(anchor, "BAD_ALIAS", "Anchor cannot be an empty string");
		}
		if (spaceBefore) node.spaceBefore = true;
		if (comment) {
			node.comment = comment;
			node.range[2] = end;
		}
		return node;
	}
	function composeAlias({ options }, { offset, source, end }, onError) {
		const alias = new Alias.Alias(source.substring(1));
		if (alias.source === "") onError(offset, "BAD_ALIAS", "Alias cannot be an empty string");
		if (alias.source.endsWith(":")) onError(offset + source.length - 1, "BAD_ALIAS", "Alias ending in : is ambiguous", true);
		const valueEnd = offset + source.length;
		const re = resolveEnd.resolveEnd(end, valueEnd, options.strict, onError);
		alias.range = [
			offset,
			valueEnd,
			re.offset
		];
		if (re.comment) alias.comment = re.comment;
		return alias;
	}
	exports.composeEmptyNode = composeEmptyNode;
	exports.composeNode = composeNode;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/compose-doc.js
var require_compose_doc = /* @__PURE__ */ __commonJSMin(((exports) => {
	var Document = require_Document();
	var composeNode = require_compose_node();
	var resolveEnd = require_resolve_end();
	var resolveProps = require_resolve_props();
	function composeDoc(options, directives, { offset, start, value, end }, onError) {
		const opts = Object.assign({ _directives: directives }, options);
		const doc = new Document.Document(void 0, opts);
		const ctx = {
			atKey: false,
			atRoot: true,
			directives: doc.directives,
			options: doc.options,
			schema: doc.schema
		};
		const props = resolveProps.resolveProps(start, {
			indicator: "doc-start",
			next: value ?? end?.[0],
			offset,
			onError,
			parentIndent: 0,
			startOnNewline: true
		});
		if (props.found) {
			doc.directives.docStart = true;
			if (value && (value.type === "block-map" || value.type === "block-seq") && !props.hasNewline) onError(props.end, "MISSING_CHAR", "Block collection cannot start on same line with directives-end marker");
		}
		doc.contents = value ? composeNode.composeNode(ctx, value, props, onError) : composeNode.composeEmptyNode(ctx, props.end, start, null, props, onError);
		const contentEnd = doc.contents.range[2];
		const re = resolveEnd.resolveEnd(end, contentEnd, false, onError);
		if (re.comment) doc.comment = re.comment;
		doc.range = [
			offset,
			contentEnd,
			re.offset
		];
		return doc;
	}
	exports.composeDoc = composeDoc;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/compose/composer.js
var require_composer = /* @__PURE__ */ __commonJSMin(((exports) => {
	var node_process = __require("process");
	var directives = require_directives();
	var Document = require_Document();
	var errors = require_errors();
	var identity = require_identity();
	var composeDoc = require_compose_doc();
	var resolveEnd = require_resolve_end();
	function getErrorPos(src) {
		if (typeof src === "number") return [src, src + 1];
		if (Array.isArray(src)) return src.length === 2 ? src : [src[0], src[1]];
		const { offset, source } = src;
		return [offset, offset + (typeof source === "string" ? source.length : 1)];
	}
	function parsePrelude(prelude) {
		let comment = "";
		let atComment = false;
		let afterEmptyLine = false;
		for (let i = 0; i < prelude.length; ++i) {
			const source = prelude[i];
			switch (source[0]) {
				case "#":
					comment += (comment === "" ? "" : afterEmptyLine ? "\n\n" : "\n") + (source.substring(1) || " ");
					atComment = true;
					afterEmptyLine = false;
					break;
				case "%":
					if (prelude[i + 1]?.[0] !== "#") i += 1;
					atComment = false;
					break;
				default:
					if (!atComment) afterEmptyLine = true;
					atComment = false;
			}
		}
		return {
			comment,
			afterEmptyLine
		};
	}
	/**
	* Compose a stream of CST nodes into a stream of YAML Documents.
	*
	* ```ts
	* import { Composer, Parser } from 'yaml'
	*
	* const src: string = ...
	* const tokens = new Parser().parse(src)
	* const docs = new Composer().compose(tokens)
	* ```
	*/
	var Composer = class {
		constructor(options = {}) {
			this.doc = null;
			this.atDirectives = false;
			this.prelude = [];
			this.errors = [];
			this.warnings = [];
			this.onError = (source, code, message, warning) => {
				const pos = getErrorPos(source);
				if (warning) this.warnings.push(new errors.YAMLWarning(pos, code, message));
				else this.errors.push(new errors.YAMLParseError(pos, code, message));
			};
			this.directives = new directives.Directives({ version: options.version || "1.2" });
			this.options = options;
		}
		decorate(doc, afterDoc) {
			const { comment, afterEmptyLine } = parsePrelude(this.prelude);
			if (comment) {
				const dc = doc.contents;
				if (afterDoc) doc.comment = doc.comment ? `${doc.comment}\n${comment}` : comment;
				else if (afterEmptyLine || doc.directives.docStart || !dc) doc.commentBefore = comment;
				else if (identity.isCollection(dc) && !dc.flow && dc.items.length > 0) {
					let it = dc.items[0];
					if (identity.isPair(it)) it = it.key;
					const cb = it.commentBefore;
					it.commentBefore = cb ? `${comment}\n${cb}` : comment;
				} else {
					const cb = dc.commentBefore;
					dc.commentBefore = cb ? `${comment}\n${cb}` : comment;
				}
			}
			if (afterDoc) {
				for (let i = 0; i < this.errors.length; ++i) doc.errors.push(this.errors[i]);
				for (let i = 0; i < this.warnings.length; ++i) doc.warnings.push(this.warnings[i]);
			} else {
				doc.errors = this.errors;
				doc.warnings = this.warnings;
			}
			this.prelude = [];
			this.errors = [];
			this.warnings = [];
		}
		/**
		* Current stream status information.
		*
		* Mostly useful at the end of input for an empty stream.
		*/
		streamInfo() {
			return {
				comment: parsePrelude(this.prelude).comment,
				directives: this.directives,
				errors: this.errors,
				warnings: this.warnings
			};
		}
		/**
		* Compose tokens into documents.
		*
		* @param forceDoc - If the stream contains no document, still emit a final document including any comments and directives that would be applied to a subsequent document.
		* @param endOffset - Should be set if `forceDoc` is also set, to set the document range end and to indicate errors correctly.
		*/
		*compose(tokens, forceDoc = false, endOffset = -1) {
			for (const token of tokens) yield* this.next(token);
			yield* this.end(forceDoc, endOffset);
		}
		/** Advance the composer by one CST token. */
		*next(token) {
			if (node_process.env.LOG_STREAM) console.dir(token, { depth: null });
			switch (token.type) {
				case "directive":
					this.directives.add(token.source, (offset, message, warning) => {
						const pos = getErrorPos(token);
						pos[0] += offset;
						this.onError(pos, "BAD_DIRECTIVE", message, warning);
					});
					this.prelude.push(token.source);
					this.atDirectives = true;
					break;
				case "document": {
					const doc = composeDoc.composeDoc(this.options, this.directives, token, this.onError);
					if (this.atDirectives && !doc.directives.docStart) this.onError(token, "MISSING_CHAR", "Missing directives-end/doc-start indicator line");
					this.decorate(doc, false);
					if (this.doc) yield this.doc;
					this.doc = doc;
					this.atDirectives = false;
					break;
				}
				case "byte-order-mark":
				case "space": break;
				case "comment":
				case "newline":
					this.prelude.push(token.source);
					break;
				case "error": {
					const msg = token.source ? `${token.message}: ${JSON.stringify(token.source)}` : token.message;
					const error = new errors.YAMLParseError(getErrorPos(token), "UNEXPECTED_TOKEN", msg);
					if (this.atDirectives || !this.doc) this.errors.push(error);
					else this.doc.errors.push(error);
					break;
				}
				case "doc-end": {
					if (!this.doc) {
						this.errors.push(new errors.YAMLParseError(getErrorPos(token), "UNEXPECTED_TOKEN", "Unexpected doc-end without preceding document"));
						break;
					}
					this.doc.directives.docEnd = true;
					const end = resolveEnd.resolveEnd(token.end, token.offset + token.source.length, this.doc.options.strict, this.onError);
					this.decorate(this.doc, true);
					if (end.comment) {
						const dc = this.doc.comment;
						this.doc.comment = dc ? `${dc}\n${end.comment}` : end.comment;
					}
					this.doc.range[2] = end.offset;
					break;
				}
				default: this.errors.push(new errors.YAMLParseError(getErrorPos(token), "UNEXPECTED_TOKEN", `Unsupported token ${token.type}`));
			}
		}
		/**
		* Call at end of input to yield any remaining document.
		*
		* @param forceDoc - If the stream contains no document, still emit a final document including any comments and directives that would be applied to a subsequent document.
		* @param endOffset - Should be set if `forceDoc` is also set, to set the document range end and to indicate errors correctly.
		*/
		*end(forceDoc = false, endOffset = -1) {
			if (this.doc) {
				this.decorate(this.doc, true);
				yield this.doc;
				this.doc = null;
			} else if (forceDoc) {
				const opts = Object.assign({ _directives: this.directives }, this.options);
				const doc = new Document.Document(void 0, opts);
				if (this.atDirectives) this.onError(endOffset, "MISSING_CHAR", "Missing directives-end indicator line");
				doc.range = [
					0,
					endOffset,
					endOffset
				];
				this.decorate(doc, false);
				yield doc;
			}
		}
	};
	exports.Composer = Composer;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/parse/cst-scalar.js
var require_cst_scalar = /* @__PURE__ */ __commonJSMin(((exports) => {
	var resolveBlockScalar = require_resolve_block_scalar();
	var resolveFlowScalar = require_resolve_flow_scalar();
	var errors = require_errors();
	var stringifyString = require_stringifyString();
	function resolveAsScalar(token, strict = true, onError) {
		if (token) {
			const _onError = (pos, code, message) => {
				const offset = typeof pos === "number" ? pos : Array.isArray(pos) ? pos[0] : pos.offset;
				if (onError) onError(offset, code, message);
				else throw new errors.YAMLParseError([offset, offset + 1], code, message);
			};
			switch (token.type) {
				case "scalar":
				case "single-quoted-scalar":
				case "double-quoted-scalar": return resolveFlowScalar.resolveFlowScalar(token, strict, _onError);
				case "block-scalar": return resolveBlockScalar.resolveBlockScalar({ options: { strict } }, token, _onError);
			}
		}
		return null;
	}
	/**
	* Create a new scalar token with `value`
	*
	* Values that represent an actual string but may be parsed as a different type should use a `type` other than `'PLAIN'`,
	* as this function does not support any schema operations and won't check for such conflicts.
	*
	* @param value The string representation of the value, which will have its content properly indented.
	* @param context.end Comments and whitespace after the end of the value, or after the block scalar header. If undefined, a newline will be added.
	* @param context.implicitKey Being within an implicit key may affect the resolved type of the token's value.
	* @param context.indent The indent level of the token.
	* @param context.inFlow Is this scalar within a flow collection? This may affect the resolved type of the token's value.
	* @param context.offset The offset position of the token.
	* @param context.type The preferred type of the scalar token. If undefined, the previous type of the `token` will be used, defaulting to `'PLAIN'`.
	*/
	function createScalarToken(value, context) {
		const { implicitKey = false, indent, inFlow = false, offset = -1, type = "PLAIN" } = context;
		const source = stringifyString.stringifyString({
			type,
			value
		}, {
			implicitKey,
			indent: indent > 0 ? " ".repeat(indent) : "",
			inFlow,
			options: {
				blockQuote: true,
				lineWidth: -1
			}
		});
		const end = context.end ?? [{
			type: "newline",
			offset: -1,
			indent,
			source: "\n"
		}];
		switch (source[0]) {
			case "|":
			case ">": {
				const he = source.indexOf("\n");
				const head = source.substring(0, he);
				const body = source.substring(he + 1) + "\n";
				const props = [{
					type: "block-scalar-header",
					offset,
					indent,
					source: head
				}];
				if (!addEndtoBlockProps(props, end)) props.push({
					type: "newline",
					offset: -1,
					indent,
					source: "\n"
				});
				return {
					type: "block-scalar",
					offset,
					indent,
					props,
					source: body
				};
			}
			case "\"": return {
				type: "double-quoted-scalar",
				offset,
				indent,
				source,
				end
			};
			case "'": return {
				type: "single-quoted-scalar",
				offset,
				indent,
				source,
				end
			};
			default: return {
				type: "scalar",
				offset,
				indent,
				source,
				end
			};
		}
	}
	/**
	* Set the value of `token` to the given string `value`, overwriting any previous contents and type that it may have.
	*
	* Best efforts are made to retain any comments previously associated with the `token`,
	* though all contents within a collection's `items` will be overwritten.
	*
	* Values that represent an actual string but may be parsed as a different type should use a `type` other than `'PLAIN'`,
	* as this function does not support any schema operations and won't check for such conflicts.
	*
	* @param token Any token. If it does not include an `indent` value, the value will be stringified as if it were an implicit key.
	* @param value The string representation of the value, which will have its content properly indented.
	* @param context.afterKey In most cases, values after a key should have an additional level of indentation.
	* @param context.implicitKey Being within an implicit key may affect the resolved type of the token's value.
	* @param context.inFlow Being within a flow collection may affect the resolved type of the token's value.
	* @param context.type The preferred type of the scalar token. If undefined, the previous type of the `token` will be used, defaulting to `'PLAIN'`.
	*/
	function setScalarValue(token, value, context = {}) {
		let { afterKey = false, implicitKey = false, inFlow = false, type } = context;
		let indent = "indent" in token ? token.indent : null;
		if (afterKey && typeof indent === "number") indent += 2;
		if (!type) switch (token.type) {
			case "single-quoted-scalar":
				type = "QUOTE_SINGLE";
				break;
			case "double-quoted-scalar":
				type = "QUOTE_DOUBLE";
				break;
			case "block-scalar": {
				const header = token.props[0];
				if (header.type !== "block-scalar-header") throw new Error("Invalid block scalar header");
				type = header.source[0] === ">" ? "BLOCK_FOLDED" : "BLOCK_LITERAL";
				break;
			}
			default: type = "PLAIN";
		}
		const source = stringifyString.stringifyString({
			type,
			value
		}, {
			implicitKey: implicitKey || indent === null,
			indent: indent !== null && indent > 0 ? " ".repeat(indent) : "",
			inFlow,
			options: {
				blockQuote: true,
				lineWidth: -1
			}
		});
		switch (source[0]) {
			case "|":
			case ">":
				setBlockScalarValue(token, source);
				break;
			case "\"":
				setFlowScalarValue(token, source, "double-quoted-scalar");
				break;
			case "'":
				setFlowScalarValue(token, source, "single-quoted-scalar");
				break;
			default: setFlowScalarValue(token, source, "scalar");
		}
	}
	function setBlockScalarValue(token, source) {
		const he = source.indexOf("\n");
		const head = source.substring(0, he);
		const body = source.substring(he + 1) + "\n";
		if (token.type === "block-scalar") {
			const header = token.props[0];
			if (header.type !== "block-scalar-header") throw new Error("Invalid block scalar header");
			header.source = head;
			token.source = body;
		} else {
			const { offset } = token;
			const indent = "indent" in token ? token.indent : -1;
			const props = [{
				type: "block-scalar-header",
				offset,
				indent,
				source: head
			}];
			if (!addEndtoBlockProps(props, "end" in token ? token.end : void 0)) props.push({
				type: "newline",
				offset: -1,
				indent,
				source: "\n"
			});
			for (const key of Object.keys(token)) if (key !== "type" && key !== "offset") delete token[key];
			Object.assign(token, {
				type: "block-scalar",
				indent,
				props,
				source: body
			});
		}
	}
	/** @returns `true` if last token is a newline */
	function addEndtoBlockProps(props, end) {
		if (end) for (const st of end) switch (st.type) {
			case "space":
			case "comment":
				props.push(st);
				break;
			case "newline":
				props.push(st);
				return true;
		}
		return false;
	}
	function setFlowScalarValue(token, source, type) {
		switch (token.type) {
			case "scalar":
			case "double-quoted-scalar":
			case "single-quoted-scalar":
				token.type = type;
				token.source = source;
				break;
			case "block-scalar": {
				const end = token.props.slice(1);
				let oa = source.length;
				if (token.props[0].type === "block-scalar-header") oa -= token.props[0].source.length;
				for (const tok of end) tok.offset += oa;
				delete token.props;
				Object.assign(token, {
					type,
					source,
					end
				});
				break;
			}
			case "block-map":
			case "block-seq": {
				const nl = {
					type: "newline",
					offset: token.offset + source.length,
					indent: token.indent,
					source: "\n"
				};
				delete token.items;
				Object.assign(token, {
					type,
					source,
					end: [nl]
				});
				break;
			}
			default: {
				const indent = "indent" in token ? token.indent : -1;
				const end = "end" in token && Array.isArray(token.end) ? token.end.filter((st) => st.type === "space" || st.type === "comment" || st.type === "newline") : [];
				for (const key of Object.keys(token)) if (key !== "type" && key !== "offset") delete token[key];
				Object.assign(token, {
					type,
					indent,
					source,
					end
				});
			}
		}
	}
	exports.createScalarToken = createScalarToken;
	exports.resolveAsScalar = resolveAsScalar;
	exports.setScalarValue = setScalarValue;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/parse/cst-stringify.js
var require_cst_stringify = /* @__PURE__ */ __commonJSMin(((exports) => {
	/**
	* Stringify a CST document, token, or collection item
	*
	* Fair warning: This applies no validation whatsoever, and
	* simply concatenates the sources in their logical order.
	*/
	const stringify = (cst) => "type" in cst ? stringifyToken(cst) : stringifyItem(cst);
	function stringifyToken(token) {
		switch (token.type) {
			case "block-scalar": {
				let res = "";
				for (const tok of token.props) res += stringifyToken(tok);
				return res + token.source;
			}
			case "block-map":
			case "block-seq": {
				let res = "";
				for (const item of token.items) res += stringifyItem(item);
				return res;
			}
			case "flow-collection": {
				let res = token.start.source;
				for (const item of token.items) res += stringifyItem(item);
				for (const st of token.end) res += st.source;
				return res;
			}
			case "document": {
				let res = stringifyItem(token);
				if (token.end) for (const st of token.end) res += st.source;
				return res;
			}
			default: {
				let res = token.source;
				if ("end" in token && token.end) for (const st of token.end) res += st.source;
				return res;
			}
		}
	}
	function stringifyItem({ start, key, sep, value }) {
		let res = "";
		for (const st of start) res += st.source;
		if (key) res += stringifyToken(key);
		if (sep) for (const st of sep) res += st.source;
		if (value) res += stringifyToken(value);
		return res;
	}
	exports.stringify = stringify;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/parse/cst-visit.js
var require_cst_visit = /* @__PURE__ */ __commonJSMin(((exports) => {
	const BREAK = Symbol("break visit");
	const SKIP = Symbol("skip children");
	const REMOVE = Symbol("remove item");
	/**
	* Apply a visitor to a CST document or item.
	*
	* Walks through the tree (depth-first) starting from the root, calling a
	* `visitor` function with two arguments when entering each item:
	*   - `item`: The current item, which included the following members:
	*     - `start: SourceToken[]` – Source tokens before the key or value,
	*       possibly including its anchor or tag.
	*     - `key?: Token | null` – Set for pair values. May then be `null`, if
	*       the key before the `:` separator is empty.
	*     - `sep?: SourceToken[]` – Source tokens between the key and the value,
	*       which should include the `:` map value indicator if `value` is set.
	*     - `value?: Token` – The value of a sequence item, or of a map pair.
	*   - `path`: The steps from the root to the current node, as an array of
	*     `['key' | 'value', number]` tuples.
	*
	* The return value of the visitor may be used to control the traversal:
	*   - `undefined` (default): Do nothing and continue
	*   - `visit.SKIP`: Do not visit the children of this token, continue with
	*      next sibling
	*   - `visit.BREAK`: Terminate traversal completely
	*   - `visit.REMOVE`: Remove the current item, then continue with the next one
	*   - `number`: Set the index of the next step. This is useful especially if
	*     the index of the current token has changed.
	*   - `function`: Define the next visitor for this item. After the original
	*     visitor is called on item entry, next visitors are called after handling
	*     a non-empty `key` and when exiting the item.
	*/
	function visit(cst, visitor) {
		if ("type" in cst && cst.type === "document") cst = {
			start: cst.start,
			value: cst.value
		};
		_visit(Object.freeze([]), cst, visitor);
	}
	/** Terminate visit traversal completely */
	visit.BREAK = BREAK;
	/** Do not visit the children of the current item */
	visit.SKIP = SKIP;
	/** Remove the current item */
	visit.REMOVE = REMOVE;
	/** Find the item at `path` from `cst` as the root */
	visit.itemAtPath = (cst, path) => {
		let item = cst;
		for (const [field, index] of path) {
			const tok = item?.[field];
			if (tok && "items" in tok) item = tok.items[index];
			else return void 0;
		}
		return item;
	};
	/**
	* Get the immediate parent collection of the item at `path` from `cst` as the root.
	*
	* Throws an error if the collection is not found, which should never happen if the item itself exists.
	*/
	visit.parentCollection = (cst, path) => {
		const parent = visit.itemAtPath(cst, path.slice(0, -1));
		const field = path[path.length - 1][0];
		const coll = parent?.[field];
		if (coll && "items" in coll) return coll;
		throw new Error("Parent collection not found");
	};
	function _visit(path, item, visitor) {
		let ctrl = visitor(item, path);
		if (typeof ctrl === "symbol") return ctrl;
		for (const field of ["key", "value"]) {
			const token = item[field];
			if (token && "items" in token) {
				for (let i = 0; i < token.items.length; ++i) {
					const ci = _visit(Object.freeze(path.concat([[field, i]])), token.items[i], visitor);
					if (typeof ci === "number") i = ci - 1;
					else if (ci === BREAK) return BREAK;
					else if (ci === REMOVE) {
						token.items.splice(i, 1);
						i -= 1;
					}
				}
				if (typeof ctrl === "function" && field === "key") ctrl = ctrl(item, path);
			}
		}
		return typeof ctrl === "function" ? ctrl(item, path) : ctrl;
	}
	exports.visit = visit;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/parse/cst.js
var require_cst = /* @__PURE__ */ __commonJSMin(((exports) => {
	var cstScalar = require_cst_scalar();
	var cstStringify = require_cst_stringify();
	var cstVisit = require_cst_visit();
	/** The byte order mark */
	const BOM = "﻿";
	/** Start of doc-mode */
	const DOCUMENT = "";
	/** Unexpected end of flow-mode */
	const FLOW_END = "";
	/** Next token is a scalar value */
	const SCALAR = "";
	/** @returns `true` if `token` is a flow or block collection */
	const isCollection = (token) => !!token && "items" in token;
	/** @returns `true` if `token` is a flow or block scalar; not an alias */
	const isScalar = (token) => !!token && (token.type === "scalar" || token.type === "single-quoted-scalar" || token.type === "double-quoted-scalar" || token.type === "block-scalar");
	/* istanbul ignore next */
	/** Get a printable representation of a lexer token */
	function prettyToken(token) {
		switch (token) {
			case BOM: return "<BOM>";
			case DOCUMENT: return "<DOC>";
			case FLOW_END: return "<FLOW_END>";
			case SCALAR: return "<SCALAR>";
			default: return JSON.stringify(token);
		}
	}
	/** Identify the type of a lexer token. May return `null` for unknown tokens. */
	function tokenType(source) {
		switch (source) {
			case BOM: return "byte-order-mark";
			case DOCUMENT: return "doc-mode";
			case FLOW_END: return "flow-error-end";
			case SCALAR: return "scalar";
			case "---": return "doc-start";
			case "...": return "doc-end";
			case "":
			case "\n":
			case "\r\n": return "newline";
			case "-": return "seq-item-ind";
			case "?": return "explicit-key-ind";
			case ":": return "map-value-ind";
			case "{": return "flow-map-start";
			case "}": return "flow-map-end";
			case "[": return "flow-seq-start";
			case "]": return "flow-seq-end";
			case ",": return "comma";
		}
		switch (source[0]) {
			case " ":
			case "	": return "space";
			case "#": return "comment";
			case "%": return "directive-line";
			case "*": return "alias";
			case "&": return "anchor";
			case "!": return "tag";
			case "'": return "single-quoted-scalar";
			case "\"": return "double-quoted-scalar";
			case "|":
			case ">": return "block-scalar-header";
		}
		return null;
	}
	exports.createScalarToken = cstScalar.createScalarToken;
	exports.resolveAsScalar = cstScalar.resolveAsScalar;
	exports.setScalarValue = cstScalar.setScalarValue;
	exports.stringify = cstStringify.stringify;
	exports.visit = cstVisit.visit;
	exports.BOM = BOM;
	exports.DOCUMENT = DOCUMENT;
	exports.FLOW_END = FLOW_END;
	exports.SCALAR = SCALAR;
	exports.isCollection = isCollection;
	exports.isScalar = isScalar;
	exports.prettyToken = prettyToken;
	exports.tokenType = tokenType;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/parse/lexer.js
var require_lexer = /* @__PURE__ */ __commonJSMin(((exports) => {
	var cst = require_cst();
	function isEmpty(ch) {
		switch (ch) {
			case void 0:
			case " ":
			case "\n":
			case "\r":
			case "	": return true;
			default: return false;
		}
	}
	const hexDigits = /* @__PURE__ */ new Set("0123456789ABCDEFabcdef");
	const tagChars = /* @__PURE__ */ new Set("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-#;/?:@&=+$_.!~*'()");
	const flowIndicatorChars = /* @__PURE__ */ new Set(",[]{}");
	const invalidAnchorChars = /* @__PURE__ */ new Set(" ,[]{}\n\r	");
	const isNotAnchorChar = (ch) => !ch || invalidAnchorChars.has(ch);
	/**
	* Splits an input string into lexical tokens, i.e. smaller strings that are
	* easily identifiable by `tokens.tokenType()`.
	*
	* Lexing starts always in a "stream" context. Incomplete input may be buffered
	* until a complete token can be emitted.
	*
	* In addition to slices of the original input, the following control characters
	* may also be emitted:
	*
	* - `\x02` (Start of Text): A document starts with the next token
	* - `\x18` (Cancel): Unexpected end of flow-mode (indicates an error)
	* - `\x1f` (Unit Separator): Next token is a scalar value
	* - `\u{FEFF}` (Byte order mark): Emitted separately outside documents
	*/
	var Lexer = class {
		constructor() {
			/**
			* Flag indicating whether the end of the current buffer marks the end of
			* all input
			*/
			this.atEnd = false;
			/**
			* Explicit indent set in block scalar header, as an offset from the current
			* minimum indent, so e.g. set to 1 from a header `|2+`. Set to -1 if not
			* explicitly set.
			*/
			this.blockScalarIndent = -1;
			/**
			* Block scalars that include a + (keep) chomping indicator in their header
			* include trailing empty lines, which are otherwise excluded from the
			* scalar's contents.
			*/
			this.blockScalarKeep = false;
			/** Current input */
			this.buffer = "";
			/**
			* Flag noting whether the map value indicator : can immediately follow this
			* node within a flow context.
			*/
			this.flowKey = false;
			/** Count of surrounding flow collection levels. */
			this.flowLevel = 0;
			/**
			* Minimum level of indentation required for next lines to be parsed as a
			* part of the current scalar value.
			*/
			this.indentNext = 0;
			/** Indentation level of the current line. */
			this.indentValue = 0;
			/** Position of the next \n character. */
			this.lineEndPos = null;
			/** Stores the state of the lexer if reaching the end of incpomplete input */
			this.next = null;
			/** A pointer to `buffer`; the current position of the lexer. */
			this.pos = 0;
		}
		/**
		* Generate YAML tokens from the `source` string. If `incomplete`,
		* a part of the last line may be left as a buffer for the next call.
		*
		* @returns A generator of lexical tokens
		*/
		*lex(source, incomplete = false) {
			if (source) {
				if (typeof source !== "string") throw TypeError("source is not a string");
				this.buffer = this.buffer ? this.buffer + source : source;
				this.lineEndPos = null;
			}
			this.atEnd = !incomplete;
			let next = this.next ?? "stream";
			while (next && (incomplete || this.hasChars(1))) next = yield* this.parseNext(next);
		}
		atLineEnd() {
			let i = this.pos;
			let ch = this.buffer[i];
			while (ch === " " || ch === "	") ch = this.buffer[++i];
			if (!ch || ch === "#" || ch === "\n") return true;
			if (ch === "\r") return this.buffer[i + 1] === "\n";
			return false;
		}
		charAt(n) {
			return this.buffer[this.pos + n];
		}
		continueScalar(offset) {
			let ch = this.buffer[offset];
			if (this.indentNext > 0) {
				let indent = 0;
				while (ch === " ") ch = this.buffer[++indent + offset];
				if (ch === "\r") {
					const next = this.buffer[indent + offset + 1];
					if (next === "\n" || !next && !this.atEnd) return offset + indent + 1;
				}
				return ch === "\n" || indent >= this.indentNext || !ch && !this.atEnd ? offset + indent : -1;
			}
			if (ch === "-" || ch === ".") {
				const dt = this.buffer.substr(offset, 3);
				if ((dt === "---" || dt === "...") && isEmpty(this.buffer[offset + 3])) return -1;
			}
			return offset;
		}
		getLine() {
			let end = this.lineEndPos;
			if (typeof end !== "number" || end !== -1 && end < this.pos) {
				end = this.buffer.indexOf("\n", this.pos);
				this.lineEndPos = end;
			}
			if (end === -1) return this.atEnd ? this.buffer.substring(this.pos) : null;
			if (this.buffer[end - 1] === "\r") end -= 1;
			return this.buffer.substring(this.pos, end);
		}
		hasChars(n) {
			return this.pos + n <= this.buffer.length;
		}
		setNext(state) {
			this.buffer = this.buffer.substring(this.pos);
			this.pos = 0;
			this.lineEndPos = null;
			this.next = state;
			return null;
		}
		peek(n) {
			return this.buffer.substr(this.pos, n);
		}
		*parseNext(next) {
			switch (next) {
				case "stream": return yield* this.parseStream();
				case "line-start": return yield* this.parseLineStart();
				case "block-start": return yield* this.parseBlockStart();
				case "doc": return yield* this.parseDocument();
				case "flow": return yield* this.parseFlowCollection();
				case "quoted-scalar": return yield* this.parseQuotedScalar();
				case "block-scalar": return yield* this.parseBlockScalar();
				case "plain-scalar": return yield* this.parsePlainScalar();
			}
		}
		*parseStream() {
			let line = this.getLine();
			if (line === null) return this.setNext("stream");
			if (line[0] === cst.BOM) {
				yield* this.pushCount(1);
				line = line.substring(1);
			}
			if (line[0] === "%") {
				let dirEnd = line.length;
				let cs = line.indexOf("#");
				while (cs !== -1) {
					const ch = line[cs - 1];
					if (ch === " " || ch === "	") {
						dirEnd = cs - 1;
						break;
					} else cs = line.indexOf("#", cs + 1);
				}
				while (true) {
					const ch = line[dirEnd - 1];
					if (ch === " " || ch === "	") dirEnd -= 1;
					else break;
				}
				const n = (yield* this.pushCount(dirEnd)) + (yield* this.pushSpaces(true));
				yield* this.pushCount(line.length - n);
				this.pushNewline();
				return "stream";
			}
			if (this.atLineEnd()) {
				const sp = yield* this.pushSpaces(true);
				yield* this.pushCount(line.length - sp);
				yield* this.pushNewline();
				return "stream";
			}
			yield cst.DOCUMENT;
			return yield* this.parseLineStart();
		}
		*parseLineStart() {
			const ch = this.charAt(0);
			if (!ch && !this.atEnd) return this.setNext("line-start");
			if (ch === "-" || ch === ".") {
				if (!this.atEnd && !this.hasChars(4)) return this.setNext("line-start");
				const s = this.peek(3);
				if ((s === "---" || s === "...") && isEmpty(this.charAt(3))) {
					yield* this.pushCount(3);
					this.indentValue = 0;
					this.indentNext = 0;
					return s === "---" ? "doc" : "stream";
				}
			}
			this.indentValue = yield* this.pushSpaces(false);
			if (this.indentNext > this.indentValue && !isEmpty(this.charAt(1))) this.indentNext = this.indentValue;
			return yield* this.parseBlockStart();
		}
		*parseBlockStart() {
			const [ch0, ch1] = this.peek(2);
			if (!ch1 && !this.atEnd) return this.setNext("block-start");
			if ((ch0 === "-" || ch0 === "?" || ch0 === ":") && isEmpty(ch1)) {
				const n = (yield* this.pushCount(1)) + (yield* this.pushSpaces(true));
				this.indentNext = this.indentValue + 1;
				this.indentValue += n;
				return "block-start";
			}
			return "doc";
		}
		*parseDocument() {
			yield* this.pushSpaces(true);
			const line = this.getLine();
			if (line === null) return this.setNext("doc");
			let n = yield* this.pushIndicators();
			switch (line[n]) {
				case "#": yield* this.pushCount(line.length - n);
				case void 0:
					yield* this.pushNewline();
					return yield* this.parseLineStart();
				case "{":
				case "[":
					yield* this.pushCount(1);
					this.flowKey = false;
					this.flowLevel = 1;
					return "flow";
				case "}":
				case "]":
					yield* this.pushCount(1);
					return "doc";
				case "*":
					yield* this.pushUntil(isNotAnchorChar);
					return "doc";
				case "\"":
				case "'": return yield* this.parseQuotedScalar();
				case "|":
				case ">":
					n += yield* this.parseBlockScalarHeader();
					n += yield* this.pushSpaces(true);
					yield* this.pushCount(line.length - n);
					yield* this.pushNewline();
					return yield* this.parseBlockScalar();
				default: return yield* this.parsePlainScalar();
			}
		}
		*parseFlowCollection() {
			let nl, sp;
			let indent = -1;
			do {
				nl = yield* this.pushNewline();
				if (nl > 0) {
					sp = yield* this.pushSpaces(false);
					this.indentValue = indent = sp;
				} else sp = 0;
				sp += yield* this.pushSpaces(true);
			} while (nl + sp > 0);
			const line = this.getLine();
			if (line === null) return this.setNext("flow");
			if (indent !== -1 && indent < this.indentNext && line[0] !== "#" || indent === 0 && (line.startsWith("---") || line.startsWith("...")) && isEmpty(line[3])) {
				if (!(indent === this.indentNext - 1 && this.flowLevel === 1 && (line[0] === "]" || line[0] === "}"))) {
					this.flowLevel = 0;
					yield cst.FLOW_END;
					return yield* this.parseLineStart();
				}
			}
			let n = 0;
			while (line[n] === ",") {
				n += yield* this.pushCount(1);
				n += yield* this.pushSpaces(true);
				this.flowKey = false;
			}
			n += yield* this.pushIndicators();
			switch (line[n]) {
				case void 0: return "flow";
				case "#":
					yield* this.pushCount(line.length - n);
					return "flow";
				case "{":
				case "[":
					yield* this.pushCount(1);
					this.flowKey = false;
					this.flowLevel += 1;
					return "flow";
				case "}":
				case "]":
					yield* this.pushCount(1);
					this.flowKey = true;
					this.flowLevel -= 1;
					return this.flowLevel ? "flow" : "doc";
				case "*":
					yield* this.pushUntil(isNotAnchorChar);
					return "flow";
				case "\"":
				case "'":
					this.flowKey = true;
					return yield* this.parseQuotedScalar();
				case ":": {
					const next = this.charAt(1);
					if (this.flowKey || isEmpty(next) || next === ",") {
						this.flowKey = false;
						yield* this.pushCount(1);
						yield* this.pushSpaces(true);
						return "flow";
					}
				}
				default:
					this.flowKey = false;
					return yield* this.parsePlainScalar();
			}
		}
		*parseQuotedScalar() {
			const quote = this.charAt(0);
			let end = this.buffer.indexOf(quote, this.pos + 1);
			if (quote === "'") while (end !== -1 && this.buffer[end + 1] === "'") end = this.buffer.indexOf("'", end + 2);
			else while (end !== -1) {
				let n = 0;
				while (this.buffer[end - 1 - n] === "\\") n += 1;
				if (n % 2 === 0) break;
				end = this.buffer.indexOf("\"", end + 1);
			}
			const qb = this.buffer.substring(0, end);
			let nl = qb.indexOf("\n", this.pos);
			if (nl !== -1) {
				while (nl !== -1) {
					const cs = this.continueScalar(nl + 1);
					if (cs === -1) break;
					nl = qb.indexOf("\n", cs);
				}
				if (nl !== -1) end = nl - (qb[nl - 1] === "\r" ? 2 : 1);
			}
			if (end === -1) {
				if (!this.atEnd) return this.setNext("quoted-scalar");
				end = this.buffer.length;
			}
			yield* this.pushToIndex(end + 1, false);
			return this.flowLevel ? "flow" : "doc";
		}
		*parseBlockScalarHeader() {
			this.blockScalarIndent = -1;
			this.blockScalarKeep = false;
			let i = this.pos;
			while (true) {
				const ch = this.buffer[++i];
				if (ch === "+") this.blockScalarKeep = true;
				else if (ch > "0" && ch <= "9") this.blockScalarIndent = Number(ch) - 1;
				else if (ch !== "-") break;
			}
			return yield* this.pushUntil((ch) => isEmpty(ch) || ch === "#");
		}
		*parseBlockScalar() {
			let nl = this.pos - 1;
			let indent = 0;
			let ch;
			loop: for (let i = this.pos; ch = this.buffer[i]; ++i) switch (ch) {
				case " ":
					indent += 1;
					break;
				case "\n":
					nl = i;
					indent = 0;
					break;
				case "\r": {
					const next = this.buffer[i + 1];
					if (!next && !this.atEnd) return this.setNext("block-scalar");
					if (next === "\n") break;
				}
				default: break loop;
			}
			if (!ch && !this.atEnd) return this.setNext("block-scalar");
			if (indent >= this.indentNext) {
				if (this.blockScalarIndent === -1) this.indentNext = indent;
				else this.indentNext = this.blockScalarIndent + (this.indentNext === 0 ? 1 : this.indentNext);
				do {
					const cs = this.continueScalar(nl + 1);
					if (cs === -1) break;
					nl = this.buffer.indexOf("\n", cs);
				} while (nl !== -1);
				if (nl === -1) {
					if (!this.atEnd) return this.setNext("block-scalar");
					nl = this.buffer.length;
				}
			}
			let i = nl + 1;
			ch = this.buffer[i];
			while (ch === " ") ch = this.buffer[++i];
			if (ch === "	") {
				while (ch === "	" || ch === " " || ch === "\r" || ch === "\n") ch = this.buffer[++i];
				nl = i - 1;
			} else if (!this.blockScalarKeep) do {
				let i = nl - 1;
				let ch = this.buffer[i];
				if (ch === "\r") ch = this.buffer[--i];
				const lastChar = i;
				while (ch === " ") ch = this.buffer[--i];
				if (ch === "\n" && i >= this.pos && i + 1 + indent > lastChar) nl = i;
				else break;
			} while (true);
			yield cst.SCALAR;
			yield* this.pushToIndex(nl + 1, true);
			return yield* this.parseLineStart();
		}
		*parsePlainScalar() {
			const inFlow = this.flowLevel > 0;
			let end = this.pos - 1;
			let i = this.pos - 1;
			let ch;
			while (ch = this.buffer[++i]) if (ch === ":") {
				const next = this.buffer[i + 1];
				if (isEmpty(next) || inFlow && flowIndicatorChars.has(next)) break;
				end = i;
			} else if (isEmpty(ch)) {
				let next = this.buffer[i + 1];
				if (ch === "\r") {
					if (next === "\n") {
						i += 1;
						ch = "\n";
						next = this.buffer[i + 1];
					} else end = i;
				}
				if (next === "#" || inFlow && flowIndicatorChars.has(next)) break;
				if (ch === "\n") {
					const cs = this.continueScalar(i + 1);
					if (cs === -1) break;
					i = Math.max(i, cs - 2);
				}
			} else {
				if (inFlow && flowIndicatorChars.has(ch)) break;
				end = i;
			}
			if (!ch && !this.atEnd) return this.setNext("plain-scalar");
			yield cst.SCALAR;
			yield* this.pushToIndex(end + 1, true);
			return inFlow ? "flow" : "doc";
		}
		*pushCount(n) {
			if (n > 0) {
				yield this.buffer.substr(this.pos, n);
				this.pos += n;
				return n;
			}
			return 0;
		}
		*pushToIndex(i, allowEmpty) {
			const s = this.buffer.slice(this.pos, i);
			if (s) {
				yield s;
				this.pos += s.length;
				return s.length;
			} else if (allowEmpty) yield "";
			return 0;
		}
		*pushIndicators() {
			let n = 0;
			loop: while (true) {
				switch (this.charAt(0)) {
					case "!":
						n += yield* this.pushTag();
						n += yield* this.pushSpaces(true);
						continue loop;
					case "&":
						n += yield* this.pushUntil(isNotAnchorChar);
						n += yield* this.pushSpaces(true);
						continue loop;
					case "-":
					case "?":
					case ":": {
						const inFlow = this.flowLevel > 0;
						const ch1 = this.charAt(1);
						if (isEmpty(ch1) || inFlow && flowIndicatorChars.has(ch1)) {
							if (!inFlow) this.indentNext = this.indentValue + 1;
							else if (this.flowKey) this.flowKey = false;
							n += yield* this.pushCount(1);
							n += yield* this.pushSpaces(true);
							continue loop;
						}
					}
				}
				break loop;
			}
			return n;
		}
		*pushTag() {
			if (this.charAt(1) === "<") {
				let i = this.pos + 2;
				let ch = this.buffer[i];
				while (!isEmpty(ch) && ch !== ">") ch = this.buffer[++i];
				return yield* this.pushToIndex(ch === ">" ? i + 1 : i, false);
			} else {
				let i = this.pos + 1;
				let ch = this.buffer[i];
				while (ch) if (tagChars.has(ch)) ch = this.buffer[++i];
				else if (ch === "%" && hexDigits.has(this.buffer[i + 1]) && hexDigits.has(this.buffer[i + 2])) ch = this.buffer[i += 3];
				else break;
				return yield* this.pushToIndex(i, false);
			}
		}
		*pushNewline() {
			const ch = this.buffer[this.pos];
			if (ch === "\n") return yield* this.pushCount(1);
			else if (ch === "\r" && this.charAt(1) === "\n") return yield* this.pushCount(2);
			else return 0;
		}
		*pushSpaces(allowTabs) {
			let i = this.pos - 1;
			let ch;
			do
				ch = this.buffer[++i];
			while (ch === " " || allowTabs && ch === "	");
			const n = i - this.pos;
			if (n > 0) {
				yield this.buffer.substr(this.pos, n);
				this.pos = i;
			}
			return n;
		}
		*pushUntil(test) {
			let i = this.pos;
			let ch = this.buffer[i];
			while (!test(ch)) ch = this.buffer[++i];
			return yield* this.pushToIndex(i, false);
		}
	};
	exports.Lexer = Lexer;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/parse/line-counter.js
var require_line_counter = /* @__PURE__ */ __commonJSMin(((exports) => {
	/**
	* Tracks newlines during parsing in order to provide an efficient API for
	* determining the one-indexed `{ line, col }` position for any offset
	* within the input.
	*/
	var LineCounter = class {
		constructor() {
			this.lineStarts = [];
			/**
			* Should be called in ascending order. Otherwise, call
			* `lineCounter.lineStarts.sort()` before calling `linePos()`.
			*/
			this.addNewLine = (offset) => this.lineStarts.push(offset);
			/**
			* Performs a binary search and returns the 1-indexed { line, col }
			* position of `offset`. If `line === 0`, `addNewLine` has never been
			* called or `offset` is before the first known newline.
			*/
			this.linePos = (offset) => {
				let low = 0;
				let high = this.lineStarts.length;
				while (low < high) {
					const mid = low + high >> 1;
					if (this.lineStarts[mid] < offset) low = mid + 1;
					else high = mid;
				}
				if (this.lineStarts[low] === offset) return {
					line: low + 1,
					col: 1
				};
				if (low === 0) return {
					line: 0,
					col: offset
				};
				const start = this.lineStarts[low - 1];
				return {
					line: low,
					col: offset - start + 1
				};
			};
		}
	};
	exports.LineCounter = LineCounter;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/parse/parser.js
var require_parser = /* @__PURE__ */ __commonJSMin(((exports) => {
	var node_process = __require("process");
	var cst = require_cst();
	var lexer = require_lexer();
	function includesToken(list, type) {
		for (let i = 0; i < list.length; ++i) if (list[i].type === type) return true;
		return false;
	}
	function findNonEmptyIndex(list) {
		for (let i = 0; i < list.length; ++i) switch (list[i].type) {
			case "space":
			case "comment":
			case "newline": break;
			default: return i;
		}
		return -1;
	}
	function isFlowToken(token) {
		switch (token?.type) {
			case "alias":
			case "scalar":
			case "single-quoted-scalar":
			case "double-quoted-scalar":
			case "flow-collection": return true;
			default: return false;
		}
	}
	function getPrevProps(parent) {
		switch (parent.type) {
			case "document": return parent.start;
			case "block-map": {
				const it = parent.items[parent.items.length - 1];
				return it.sep ?? it.start;
			}
			case "block-seq": return parent.items[parent.items.length - 1].start;
			/* istanbul ignore next should not happen */
			default: return [];
		}
	}
	/** Note: May modify input array */
	function getFirstKeyStartProps(prev) {
		if (prev.length === 0) return [];
		let i = prev.length;
		loop: while (--i >= 0) switch (prev[i].type) {
			case "doc-start":
			case "explicit-key-ind":
			case "map-value-ind":
			case "seq-item-ind":
			case "newline": break loop;
		}
		while (prev[++i]?.type === "space");
		return prev.splice(i, prev.length);
	}
	function arrayPushArray(target, source) {
		if (source.length < 1e5) Array.prototype.push.apply(target, source);
		else for (let i = 0; i < source.length; ++i) target.push(source[i]);
	}
	function fixFlowSeqItems(fc) {
		if (fc.start.type === "flow-seq-start") {
			for (const it of fc.items) if (it.sep && !it.value && !includesToken(it.start, "explicit-key-ind") && !includesToken(it.sep, "map-value-ind")) {
				if (it.key) it.value = it.key;
				delete it.key;
				if (isFlowToken(it.value)) {
					if (it.value.end) arrayPushArray(it.value.end, it.sep);
					else it.value.end = it.sep;
				} else arrayPushArray(it.start, it.sep);
				delete it.sep;
			}
		}
	}
	/**
	* A YAML concrete syntax tree (CST) parser
	*
	* ```ts
	* const src: string = ...
	* for (const token of new Parser().parse(src)) {
	*   // token: Token
	* }
	* ```
	*
	* To use the parser with a user-provided lexer:
	*
	* ```ts
	* function* parse(source: string, lexer: Lexer) {
	*   const parser = new Parser()
	*   for (const lexeme of lexer.lex(source))
	*     yield* parser.next(lexeme)
	*   yield* parser.end()
	* }
	*
	* const src: string = ...
	* const lexer = new Lexer()
	* for (const token of parse(src, lexer)) {
	*   // token: Token
	* }
	* ```
	*/
	var Parser = class {
		/**
		* @param onNewLine - If defined, called separately with the start position of
		*   each new line (in `parse()`, including the start of input).
		*/
		constructor(onNewLine) {
			/** If true, space and sequence indicators count as indentation */
			this.atNewLine = true;
			/** If true, next token is a scalar value */
			this.atScalar = false;
			/** Current indentation level */
			this.indent = 0;
			/** Current offset since the start of parsing */
			this.offset = 0;
			/** On the same line with a block map key */
			this.onKeyLine = false;
			/** Top indicates the node that's currently being built */
			this.stack = [];
			/** The source of the current token, set in parse() */
			this.source = "";
			/** The type of the current token, set in parse() */
			this.type = "";
			this.lexer = new lexer.Lexer();
			this.onNewLine = onNewLine;
		}
		/**
		* Parse `source` as a YAML stream.
		* If `incomplete`, a part of the last line may be left as a buffer for the next call.
		*
		* Errors are not thrown, but yielded as `{ type: 'error', message }` tokens.
		*
		* @returns A generator of tokens representing each directive, document, and other structure.
		*/
		*parse(source, incomplete = false) {
			if (this.onNewLine && this.offset === 0) this.onNewLine(0);
			for (const lexeme of this.lexer.lex(source, incomplete)) yield* this.next(lexeme);
			if (!incomplete) yield* this.end();
		}
		/**
		* Advance the parser by the `source` of one lexical token.
		*/
		*next(source) {
			this.source = source;
			if (node_process.env.LOG_TOKENS) console.log("|", cst.prettyToken(source));
			if (this.atScalar) {
				this.atScalar = false;
				yield* this.step();
				this.offset += source.length;
				return;
			}
			const type = cst.tokenType(source);
			if (!type) {
				const message = `Not a YAML token: ${source}`;
				yield* this.pop({
					type: "error",
					offset: this.offset,
					message,
					source
				});
				this.offset += source.length;
			} else if (type === "scalar") {
				this.atNewLine = false;
				this.atScalar = true;
				this.type = "scalar";
			} else {
				this.type = type;
				yield* this.step();
				switch (type) {
					case "newline":
						this.atNewLine = true;
						this.indent = 0;
						if (this.onNewLine) this.onNewLine(this.offset + source.length);
						break;
					case "space":
						if (this.atNewLine && source[0] === " ") this.indent += source.length;
						break;
					case "explicit-key-ind":
					case "map-value-ind":
					case "seq-item-ind":
						if (this.atNewLine) this.indent += source.length;
						break;
					case "doc-mode":
					case "flow-error-end": return;
					default: this.atNewLine = false;
				}
				this.offset += source.length;
			}
		}
		/** Call at end of input to push out any remaining constructions */
		*end() {
			while (this.stack.length > 0) yield* this.pop();
		}
		get sourceToken() {
			return {
				type: this.type,
				offset: this.offset,
				indent: this.indent,
				source: this.source
			};
		}
		*step() {
			const top = this.peek(1);
			if (this.type === "doc-end" && top?.type !== "doc-end") {
				while (this.stack.length > 0) yield* this.pop();
				this.stack.push({
					type: "doc-end",
					offset: this.offset,
					source: this.source
				});
				return;
			}
			if (!top) return yield* this.stream();
			switch (top.type) {
				case "document": return yield* this.document(top);
				case "alias":
				case "scalar":
				case "single-quoted-scalar":
				case "double-quoted-scalar": return yield* this.scalar(top);
				case "block-scalar": return yield* this.blockScalar(top);
				case "block-map": return yield* this.blockMap(top);
				case "block-seq": return yield* this.blockSequence(top);
				case "flow-collection": return yield* this.flowCollection(top);
				case "doc-end": return yield* this.documentEnd(top);
			}
			/* istanbul ignore next should not happen */
			yield* this.pop();
		}
		peek(n) {
			return this.stack[this.stack.length - n];
		}
		*pop(error) {
			const token = error ?? this.stack.pop();
			/* istanbul ignore if should not happen */
			if (!token) yield {
				type: "error",
				offset: this.offset,
				source: "",
				message: "Tried to pop an empty stack"
			};
			else if (this.stack.length === 0) yield token;
			else {
				const top = this.peek(1);
				if (token.type === "block-scalar") token.indent = "indent" in top ? top.indent : 0;
				else if (token.type === "flow-collection" && top.type === "document") token.indent = 0;
				if (token.type === "flow-collection") fixFlowSeqItems(token);
				switch (top.type) {
					case "document":
						top.value = token;
						break;
					case "block-scalar":
						top.props.push(token);
						break;
					case "block-map": {
						const it = top.items[top.items.length - 1];
						if (it.value) {
							top.items.push({
								start: [],
								key: token,
								sep: []
							});
							this.onKeyLine = true;
							return;
						} else if (it.sep) it.value = token;
						else {
							Object.assign(it, {
								key: token,
								sep: []
							});
							this.onKeyLine = !it.explicitKey;
							return;
						}
						break;
					}
					case "block-seq": {
						const it = top.items[top.items.length - 1];
						if (it.value) top.items.push({
							start: [],
							value: token
						});
						else it.value = token;
						break;
					}
					case "flow-collection": {
						const it = top.items[top.items.length - 1];
						if (!it || it.value) top.items.push({
							start: [],
							key: token,
							sep: []
						});
						else if (it.sep) it.value = token;
						else Object.assign(it, {
							key: token,
							sep: []
						});
						return;
					}
					/* istanbul ignore next should not happen */
					default:
						yield* this.pop();
						yield* this.pop(token);
				}
				if ((top.type === "document" || top.type === "block-map" || top.type === "block-seq") && (token.type === "block-map" || token.type === "block-seq")) {
					const last = token.items[token.items.length - 1];
					if (last && !last.sep && !last.value && last.start.length > 0 && findNonEmptyIndex(last.start) === -1 && (token.indent === 0 || last.start.every((st) => st.type !== "comment" || st.indent < token.indent))) {
						if (top.type === "document") top.end = last.start;
						else top.items.push({ start: last.start });
						token.items.splice(-1, 1);
					}
				}
			}
		}
		*stream() {
			switch (this.type) {
				case "directive-line":
					yield {
						type: "directive",
						offset: this.offset,
						source: this.source
					};
					return;
				case "byte-order-mark":
				case "space":
				case "comment":
				case "newline":
					yield this.sourceToken;
					return;
				case "doc-mode":
				case "doc-start": {
					const doc = {
						type: "document",
						offset: this.offset,
						start: []
					};
					if (this.type === "doc-start") doc.start.push(this.sourceToken);
					this.stack.push(doc);
					return;
				}
			}
			yield {
				type: "error",
				offset: this.offset,
				message: `Unexpected ${this.type} token in YAML stream`,
				source: this.source
			};
		}
		*document(doc) {
			if (doc.value) return yield* this.lineEnd(doc);
			switch (this.type) {
				case "doc-start":
					if (findNonEmptyIndex(doc.start) !== -1) {
						yield* this.pop();
						yield* this.step();
					} else doc.start.push(this.sourceToken);
					return;
				case "anchor":
				case "tag":
				case "space":
				case "comment":
				case "newline":
					doc.start.push(this.sourceToken);
					return;
			}
			const bv = this.startBlockValue(doc);
			if (bv) this.stack.push(bv);
			else yield {
				type: "error",
				offset: this.offset,
				message: `Unexpected ${this.type} token in YAML document`,
				source: this.source
			};
		}
		*scalar(scalar) {
			if (this.type === "map-value-ind") {
				const start = getFirstKeyStartProps(getPrevProps(this.peek(2)));
				let sep;
				if (scalar.end) {
					sep = scalar.end;
					sep.push(this.sourceToken);
					delete scalar.end;
				} else sep = [this.sourceToken];
				const map = {
					type: "block-map",
					offset: scalar.offset,
					indent: scalar.indent,
					items: [{
						start,
						key: scalar,
						sep
					}]
				};
				this.onKeyLine = true;
				this.stack[this.stack.length - 1] = map;
			} else yield* this.lineEnd(scalar);
		}
		*blockScalar(scalar) {
			switch (this.type) {
				case "space":
				case "comment":
				case "newline":
					scalar.props.push(this.sourceToken);
					return;
				case "scalar":
					scalar.source = this.source;
					this.atNewLine = true;
					this.indent = 0;
					if (this.onNewLine) {
						let nl = this.source.indexOf("\n") + 1;
						while (nl !== 0) {
							this.onNewLine(this.offset + nl);
							nl = this.source.indexOf("\n", nl) + 1;
						}
					}
					yield* this.pop();
					break;
				/* istanbul ignore next should not happen */
				default:
					yield* this.pop();
					yield* this.step();
			}
		}
		*blockMap(map) {
			const it = map.items[map.items.length - 1];
			switch (this.type) {
				case "newline":
					this.onKeyLine = false;
					if (it.value) {
						const end = "end" in it.value ? it.value.end : void 0;
						if ((Array.isArray(end) ? end[end.length - 1] : void 0)?.type === "comment") end?.push(this.sourceToken);
						else map.items.push({ start: [this.sourceToken] });
					} else if (it.sep) it.sep.push(this.sourceToken);
					else it.start.push(this.sourceToken);
					return;
				case "space":
				case "comment":
					if (it.value) map.items.push({ start: [this.sourceToken] });
					else if (it.sep) it.sep.push(this.sourceToken);
					else {
						if (this.atIndentedComment(it.start, map.indent)) {
							const end = map.items[map.items.length - 2]?.value?.end;
							if (Array.isArray(end)) {
								arrayPushArray(end, it.start);
								end.push(this.sourceToken);
								map.items.pop();
								return;
							}
						}
						it.start.push(this.sourceToken);
					}
					return;
			}
			if (this.indent >= map.indent) {
				const atMapIndent = !this.onKeyLine && this.indent === map.indent;
				const atNextItem = atMapIndent && (it.sep || it.explicitKey) && this.type !== "seq-item-ind";
				let start = [];
				if (atNextItem && it.sep && !it.value) {
					const nl = [];
					for (let i = 0; i < it.sep.length; ++i) {
						const st = it.sep[i];
						switch (st.type) {
							case "newline":
								nl.push(i);
								break;
							case "space": break;
							case "comment":
								if (st.indent > map.indent) nl.length = 0;
								break;
							default: nl.length = 0;
						}
					}
					if (nl.length >= 2) start = it.sep.splice(nl[1]);
				}
				switch (this.type) {
					case "anchor":
					case "tag":
						if (atNextItem || it.value) {
							start.push(this.sourceToken);
							map.items.push({ start });
							this.onKeyLine = true;
						} else if (it.sep) it.sep.push(this.sourceToken);
						else it.start.push(this.sourceToken);
						return;
					case "explicit-key-ind":
						if (!it.sep && !it.explicitKey) {
							it.start.push(this.sourceToken);
							it.explicitKey = true;
						} else if (atNextItem || it.value) {
							start.push(this.sourceToken);
							map.items.push({
								start,
								explicitKey: true
							});
						} else this.stack.push({
							type: "block-map",
							offset: this.offset,
							indent: this.indent,
							items: [{
								start: [this.sourceToken],
								explicitKey: true
							}]
						});
						this.onKeyLine = true;
						return;
					case "map-value-ind":
						if (it.explicitKey) {
							if (!it.sep) {
								if (includesToken(it.start, "newline")) Object.assign(it, {
									key: null,
									sep: [this.sourceToken]
								});
								else {
									const start = getFirstKeyStartProps(it.start);
									this.stack.push({
										type: "block-map",
										offset: this.offset,
										indent: this.indent,
										items: [{
											start,
											key: null,
											sep: [this.sourceToken]
										}]
									});
								}
							} else if (it.value) map.items.push({
								start: [],
								key: null,
								sep: [this.sourceToken]
							});
							else if (includesToken(it.sep, "map-value-ind")) this.stack.push({
								type: "block-map",
								offset: this.offset,
								indent: this.indent,
								items: [{
									start,
									key: null,
									sep: [this.sourceToken]
								}]
							});
							else if (isFlowToken(it.key) && !includesToken(it.sep, "newline")) {
								const start = getFirstKeyStartProps(it.start);
								const key = it.key;
								const sep = it.sep;
								sep.push(this.sourceToken);
								delete it.key;
								delete it.sep;
								this.stack.push({
									type: "block-map",
									offset: this.offset,
									indent: this.indent,
									items: [{
										start,
										key,
										sep
									}]
								});
							} else if (start.length > 0) it.sep = it.sep.concat(start, this.sourceToken);
							else it.sep.push(this.sourceToken);
						} else if (!it.sep) Object.assign(it, {
							key: null,
							sep: [this.sourceToken]
						});
						else if (it.value || atNextItem) map.items.push({
							start,
							key: null,
							sep: [this.sourceToken]
						});
						else if (includesToken(it.sep, "map-value-ind")) this.stack.push({
							type: "block-map",
							offset: this.offset,
							indent: this.indent,
							items: [{
								start: [],
								key: null,
								sep: [this.sourceToken]
							}]
						});
						else it.sep.push(this.sourceToken);
						this.onKeyLine = true;
						return;
					case "alias":
					case "scalar":
					case "single-quoted-scalar":
					case "double-quoted-scalar": {
						const fs = this.flowScalar(this.type);
						if (atNextItem || it.value) {
							map.items.push({
								start,
								key: fs,
								sep: []
							});
							this.onKeyLine = true;
						} else if (it.sep) this.stack.push(fs);
						else {
							Object.assign(it, {
								key: fs,
								sep: []
							});
							this.onKeyLine = true;
						}
						return;
					}
					default: {
						const bv = this.startBlockValue(map);
						if (bv) {
							if (bv.type === "block-seq") {
								if (!it.explicitKey && it.sep && !includesToken(it.sep, "newline")) {
									yield* this.pop({
										type: "error",
										offset: this.offset,
										message: "Unexpected block-seq-ind on same line with key",
										source: this.source
									});
									return;
								}
							} else if (atMapIndent) map.items.push({ start });
							this.stack.push(bv);
							return;
						}
					}
				}
			}
			yield* this.pop();
			yield* this.step();
		}
		*blockSequence(seq) {
			const it = seq.items[seq.items.length - 1];
			switch (this.type) {
				case "newline":
					if (it.value) {
						const end = "end" in it.value ? it.value.end : void 0;
						if ((Array.isArray(end) ? end[end.length - 1] : void 0)?.type === "comment") end?.push(this.sourceToken);
						else seq.items.push({ start: [this.sourceToken] });
					} else it.start.push(this.sourceToken);
					return;
				case "space":
				case "comment":
					if (it.value) seq.items.push({ start: [this.sourceToken] });
					else {
						if (this.atIndentedComment(it.start, seq.indent)) {
							const end = seq.items[seq.items.length - 2]?.value?.end;
							if (Array.isArray(end)) {
								arrayPushArray(end, it.start);
								end.push(this.sourceToken);
								seq.items.pop();
								return;
							}
						}
						it.start.push(this.sourceToken);
					}
					return;
				case "anchor":
				case "tag":
					if (it.value || this.indent <= seq.indent) break;
					it.start.push(this.sourceToken);
					return;
				case "seq-item-ind":
					if (this.indent !== seq.indent) break;
					if (it.value || includesToken(it.start, "seq-item-ind")) seq.items.push({ start: [this.sourceToken] });
					else it.start.push(this.sourceToken);
					return;
			}
			if (this.indent > seq.indent) {
				const bv = this.startBlockValue(seq);
				if (bv) {
					this.stack.push(bv);
					return;
				}
			}
			yield* this.pop();
			yield* this.step();
		}
		*flowCollection(fc) {
			const it = fc.items[fc.items.length - 1];
			if (this.type === "flow-error-end") {
				let top;
				do {
					yield* this.pop();
					top = this.peek(1);
				} while (top?.type === "flow-collection");
			} else if (fc.end.length === 0) {
				switch (this.type) {
					case "comma":
					case "explicit-key-ind":
						if (!it || it.sep) fc.items.push({ start: [this.sourceToken] });
						else it.start.push(this.sourceToken);
						return;
					case "map-value-ind":
						if (!it || it.value) fc.items.push({
							start: [],
							key: null,
							sep: [this.sourceToken]
						});
						else if (it.sep) it.sep.push(this.sourceToken);
						else Object.assign(it, {
							key: null,
							sep: [this.sourceToken]
						});
						return;
					case "space":
					case "comment":
					case "newline":
					case "anchor":
					case "tag":
						if (!it || it.value) fc.items.push({ start: [this.sourceToken] });
						else if (it.sep) it.sep.push(this.sourceToken);
						else it.start.push(this.sourceToken);
						return;
					case "alias":
					case "scalar":
					case "single-quoted-scalar":
					case "double-quoted-scalar": {
						const fs = this.flowScalar(this.type);
						if (!it || it.value) fc.items.push({
							start: [],
							key: fs,
							sep: []
						});
						else if (it.sep) this.stack.push(fs);
						else Object.assign(it, {
							key: fs,
							sep: []
						});
						return;
					}
					case "flow-map-end":
					case "flow-seq-end":
						fc.end.push(this.sourceToken);
						return;
				}
				const bv = this.startBlockValue(fc);
				/* istanbul ignore else should not happen */
				if (bv) this.stack.push(bv);
				else {
					yield* this.pop();
					yield* this.step();
				}
			} else {
				const parent = this.peek(2);
				if (parent.type === "block-map" && (this.type === "map-value-ind" && parent.indent === fc.indent || this.type === "newline" && !parent.items[parent.items.length - 1].sep)) {
					yield* this.pop();
					yield* this.step();
				} else if (this.type === "map-value-ind" && parent.type !== "flow-collection") {
					const start = getFirstKeyStartProps(getPrevProps(parent));
					fixFlowSeqItems(fc);
					const sep = fc.end.splice(1, fc.end.length);
					sep.push(this.sourceToken);
					const map = {
						type: "block-map",
						offset: fc.offset,
						indent: fc.indent,
						items: [{
							start,
							key: fc,
							sep
						}]
					};
					this.onKeyLine = true;
					this.stack[this.stack.length - 1] = map;
				} else yield* this.lineEnd(fc);
			}
		}
		flowScalar(type) {
			if (this.onNewLine) {
				let nl = this.source.indexOf("\n") + 1;
				while (nl !== 0) {
					this.onNewLine(this.offset + nl);
					nl = this.source.indexOf("\n", nl) + 1;
				}
			}
			return {
				type,
				offset: this.offset,
				indent: this.indent,
				source: this.source
			};
		}
		startBlockValue(parent) {
			switch (this.type) {
				case "alias":
				case "scalar":
				case "single-quoted-scalar":
				case "double-quoted-scalar": return this.flowScalar(this.type);
				case "block-scalar-header": return {
					type: "block-scalar",
					offset: this.offset,
					indent: this.indent,
					props: [this.sourceToken],
					source: ""
				};
				case "flow-map-start":
				case "flow-seq-start": return {
					type: "flow-collection",
					offset: this.offset,
					indent: this.indent,
					start: this.sourceToken,
					items: [],
					end: []
				};
				case "seq-item-ind": return {
					type: "block-seq",
					offset: this.offset,
					indent: this.indent,
					items: [{ start: [this.sourceToken] }]
				};
				case "explicit-key-ind": {
					this.onKeyLine = true;
					const start = getFirstKeyStartProps(getPrevProps(parent));
					start.push(this.sourceToken);
					return {
						type: "block-map",
						offset: this.offset,
						indent: this.indent,
						items: [{
							start,
							explicitKey: true
						}]
					};
				}
				case "map-value-ind": {
					this.onKeyLine = true;
					const start = getFirstKeyStartProps(getPrevProps(parent));
					return {
						type: "block-map",
						offset: this.offset,
						indent: this.indent,
						items: [{
							start,
							key: null,
							sep: [this.sourceToken]
						}]
					};
				}
			}
			return null;
		}
		atIndentedComment(start, indent) {
			if (this.type !== "comment") return false;
			if (this.indent <= indent) return false;
			return start.every((st) => st.type === "newline" || st.type === "space");
		}
		*documentEnd(docEnd) {
			if (this.type !== "doc-mode") {
				if (docEnd.end) docEnd.end.push(this.sourceToken);
				else docEnd.end = [this.sourceToken];
				if (this.type === "newline") yield* this.pop();
			}
		}
		*lineEnd(token) {
			switch (this.type) {
				case "comma":
				case "doc-start":
				case "doc-end":
				case "flow-seq-end":
				case "flow-map-end":
				case "map-value-ind":
					yield* this.pop();
					yield* this.step();
					break;
				case "newline": this.onKeyLine = false;
				default:
					if (token.end) token.end.push(this.sourceToken);
					else token.end = [this.sourceToken];
					if (this.type === "newline") yield* this.pop();
			}
		}
	};
	exports.Parser = Parser;
}));
// ../../node_modules/.pnpm/yaml@2.9.1/node_modules/yaml/dist/public-api.js
var require_public_api = /* @__PURE__ */ __commonJSMin(((exports) => {
	var composer = require_composer();
	var Document = require_Document();
	var errors = require_errors();
	var log = require_log();
	var identity = require_identity();
	var lineCounter = require_line_counter();
	var parser = require_parser();
	function parseOptions(options) {
		const prettyErrors = options.prettyErrors !== false;
		return {
			lineCounter: options.lineCounter || prettyErrors && new lineCounter.LineCounter() || null,
			prettyErrors
		};
	}
	/**
	* Parse the input as a stream of YAML documents.
	*
	* Documents should be separated from each other by `...` or `---` marker lines.
	*
	* @returns If an empty `docs` array is returned, it will be of type
	*   EmptyStream and contain additional stream information. In
	*   TypeScript, you should use `'empty' in docs` as a type guard for it.
	*/
	function parseAllDocuments(source, options = {}) {
		const { lineCounter, prettyErrors } = parseOptions(options);
		const parser$1 = new parser.Parser(lineCounter?.addNewLine);
		const composer$1 = new composer.Composer(options);
		const docs = Array.from(composer$1.compose(parser$1.parse(source)));
		if (prettyErrors && lineCounter) for (const doc of docs) {
			doc.errors.forEach(errors.prettifyError(source, lineCounter));
			doc.warnings.forEach(errors.prettifyError(source, lineCounter));
		}
		if (docs.length > 0) return docs;
		return Object.assign([], { empty: true }, composer$1.streamInfo());
	}
	/** Parse an input string into a single YAML.Document */
	function parseDocument(source, options = {}) {
		const { lineCounter, prettyErrors } = parseOptions(options);
		const parser$1 = new parser.Parser(lineCounter?.addNewLine);
		const composer$1 = new composer.Composer(options);
		let doc = null;
		for (const _doc of composer$1.compose(parser$1.parse(source), true, source.length)) if (!doc) doc = _doc;
		else if (doc.options.logLevel !== "silent") {
			doc.errors.push(new errors.YAMLParseError(_doc.range.slice(0, 2), "MULTIPLE_DOCS", "Source contains multiple documents; please use YAML.parseAllDocuments()"));
			break;
		}
		if (prettyErrors && lineCounter) {
			doc.errors.forEach(errors.prettifyError(source, lineCounter));
			doc.warnings.forEach(errors.prettifyError(source, lineCounter));
		}
		return doc;
	}
	function parse(src, reviver, options) {
		let _reviver = void 0;
		if (typeof reviver === "function") _reviver = reviver;
		else if (options === void 0 && reviver && typeof reviver === "object") options = reviver;
		const doc = parseDocument(src, options);
		if (!doc) return null;
		doc.warnings.forEach((warning) => log.warn(doc.options.logLevel, warning));
		if (doc.errors.length > 0) {
			if (doc.options.logLevel !== "silent") throw doc.errors[0];
			else doc.errors = [];
		}
		return doc.toJS(Object.assign({ reviver: _reviver }, options));
	}
	function stringify(value, replacer, options) {
		let _replacer = null;
		if (typeof replacer === "function" || Array.isArray(replacer)) _replacer = replacer;
		else if (options === void 0 && replacer) options = replacer;
		if (typeof options === "string") options = options.length;
		if (typeof options === "number") {
			const indent = Math.round(options);
			options = indent < 1 ? void 0 : indent > 8 ? { indent: 8 } : { indent };
		}
		if (value === void 0) {
			const { keepUndefined } = options ?? replacer ?? {};
			if (!keepUndefined) return void 0;
		}
		if (identity.isDocument(value) && !_replacer) return value.toString(options);
		return new Document.Document(value, _replacer, options).toString(options);
	}
	exports.parse = parse;
	exports.parseAllDocuments = parseAllDocuments;
	exports.parseDocument = parseDocument;
	exports.stringify = stringify;
}));
// src/SkillDeployer.ts
var import_dist = (/* @__PURE__ */ __commonJSMin(((exports) => {
	var composer = require_composer();
	var Document = require_Document();
	var Schema = require_Schema();
	var errors = require_errors();
	var Alias = require_Alias();
	var identity = require_identity();
	var Pair = require_Pair();
	var Scalar = require_Scalar();
	var YAMLMap = require_YAMLMap();
	var YAMLSeq = require_YAMLSeq();
	require_cst();
	var lexer = require_lexer();
	var lineCounter = require_line_counter();
	var parser = require_parser();
	var publicApi = require_public_api();
	var visit = require_visit();
	exports.Composer = composer.Composer;
	exports.Document = Document.Document;
	exports.Schema = Schema.Schema;
	exports.YAMLError = errors.YAMLError;
	exports.YAMLParseError = errors.YAMLParseError;
	exports.YAMLWarning = errors.YAMLWarning;
	exports.Alias = Alias.Alias;
	exports.isAlias = identity.isAlias;
	exports.isCollection = identity.isCollection;
	exports.isDocument = identity.isDocument;
	exports.isMap = identity.isMap;
	exports.isNode = identity.isNode;
	exports.isPair = identity.isPair;
	exports.isScalar = identity.isScalar;
	exports.isSeq = identity.isSeq;
	exports.Pair = Pair.Pair;
	exports.Scalar = Scalar.Scalar;
	exports.YAMLMap = YAMLMap.YAMLMap;
	exports.YAMLSeq = YAMLSeq.YAMLSeq;
	exports.Lexer = lexer.Lexer;
	exports.LineCounter = lineCounter.LineCounter;
	exports.Parser = parser.Parser;
	exports.parse = publicApi.parse;
	exports.parseAllDocuments = publicApi.parseAllDocuments;
	exports.parseDocument = publicApi.parseDocument;
	exports.stringify = publicApi.stringify;
	exports.visit = visit.visit;
	exports.visitAsync = visit.visitAsync;
})))();
var SkillDeployer = class extends Context.Service()("agx-opencode/SkillDeployer") {};
const makeSkillDeployer = Effect.fn("makeSkillDeployer")(function* () {
	const fs = yield* FileSystem.FileSystem;
	const path = yield* Path$2.Path;
	const crypto = yield* Crypto.Crypto;
	const scope = yield* Effect.scope;
	const deployed = /* @__PURE__ */ new Map();
	let root = null;
	const deploy = Effect.fn("SkillDeployer.deploy")(function* (tree) {
		const existing = deployed.get(tree.digest);
		if (existing !== void 0) return path.join(existing, "SKILL.md");
		if (root === null) root = yield* fs.makeTempDirectoryScoped({ prefix: "agx-opencode-" }).pipe(Effect.provideService(Scope.Scope, scope));
		const directory = path.join(root, tree.digest);
		yield* fs.makeDirectory(directory, {
			recursive: true,
			mode: 448
		});
		for (const name of tree.directories) yield* fs.makeDirectory(path.join(directory, ...name.split("/")), {
			recursive: true,
			mode: 448
		});
		for (const file of tree.files) {
			const destination = path.join(directory, ...file.path.split("/"));
			yield* fs.makeDirectory(path.dirname(destination), {
				recursive: true,
				mode: 448
			});
			yield* fs.writeFile(destination, file.content, { mode: 384 });
			if (file.executable) yield* fs.chmod(destination, 448);
		}
		deployed.set(tree.digest, directory);
		return path.join(directory, "SKILL.md");
	}, Effect.catchTag("PlatformError", (cause) => Effect.fail(new IntegrationError({
		operation: "skill deployment",
		reason: "filesystem",
		cause
	}))));
	const prepare = Effect.fn("SkillDeployer.prepare")(function* (tree) {
		const document = tree.files.find((file) => file.path === "SKILL.md");
		if (document === void 0) return yield* new IntegrationError({
			operation: "skill document",
			reason: "content"
		});
		const text = new TextDecoder().decode(document.content).replace(/^\uFEFF/, "").replace(/\r\n/g, "\n");
		let content = text;
		let frontmatter = {};
		if (text.startsWith("---\n")) {
			const end = text.indexOf("\n---", 4);
			if (end === -1) return yield* new IntegrationError({
				operation: "skill frontmatter",
				reason: "content"
			});
			frontmatter = yield* Effect.try(() => (0, import_dist.parse)(text.slice(4, end)) ?? {}).pipe(Effect.catchTag("UnknownError", (error) => {
				const cause = error.cause;
				const code = Schema$1.is(Schema$1.instanceOf(import_dist.YAMLParseError))(cause) ? cause.code : Schema$1.is(Schema$1.instanceOf(ReferenceError))(cause) && cause.message === "Excessive alias count indicates a resource exhaustion attack" ? "EXCESSIVE_ALIASES" : null;
				return code === null ? Effect.die(cause) : Effect.fail(new IntegrationError({
					operation: "skill frontmatter",
					reason: "content",
					cause: {
						kind: "yaml",
						code
					}
				}));
			}));
			content = text.slice(end + 4).replace(/^\n/, "");
		}
		const metadata = yield* Schema$1.decodeUnknownEffect(Frontmatter)(frontmatter).pipe(Effect.catchTag("SchemaError", (cause) => Effect.fail(new IntegrationError({
			operation: "skill metadata",
			reason: "content",
			cause
		}))));
		const hash = yield* crypto.digest("SHA-256", new TextEncoder().encode(tree.id)).pipe(Effect.catchTag("PlatformError", (cause) => Effect.fail(new IntegrationError({
			operation: "skill identity",
			reason: "content",
			cause
		}))));
		const id = ID.make(`agx-${tree.name}-${Hex.encode(hash).slice(0, 12)}`);
		const location = tree.files.some((file) => file.path !== "SKILL.md") || tree.directories.length > 0 ? yield* deploy(tree) : `skill://${id}/SKILL.md`;
		return Info.make({
			id,
			name: Name.make(tree.name),
			...metadata.description === void 0 ? {} : { description: metadata.description },
			autoinvoke: metadata["disable-model-invocation"] !== true,
			path: Info.fields.path.make(location),
			content
		});
	});
	return SkillDeployer.of({ prepare });
});
const SkillDeployerLive = Layer.effect(SkillDeployer, makeSkillDeployer());
// src/lease.ts
const acquireLease = Effect.fn("acquireLease")(function* (onChange) {
	const client = yield* AgxClient;
	const connection = yield* AgxConnection;
	const configuration = yield* Configuration;
	const deployer = yield* SkillDeployer;
	const crypto = yield* Crypto.Crypto;
	const lock = Semaphore.makeUnsafe(1);
	const acquireCurrent = Effect.fn("lease.acquire")(function* () {
		const target = yield* connection.discover();
		const resolve = yield* client.resolve(target, {
			directory: configuration.scopeDirectory,
			harness: configuration.harness
		});
		if (resolve.settings.length > 0) return yield* new IntegrationError({
			operation: "native settings are unsupported",
			reason: "settings"
		});
		const snapshot = yield* Schema$1.decodeUnknownEffect(Snapshot)(resolve.snapshot).pipe(Effect.catchTag("SchemaError", (cause) => Effect.fail(new IntegrationError({
			operation: "resolved snapshot",
			reason: "content",
			cause
		}))));
		const environmentInputs = {};
		for (const name of snapshot.requiredEnvironment) {
			const value = yield* Config.option(Config.String(name)).pipe(Effect.catchTag("ConfigError", (cause) => Effect.fail(new IntegrationError({
				operation: "resource environment",
				reason: "options",
				cause
			}))));
			if (Option$1.isSome(value)) environmentInputs[name] = value.value;
		}
		const raw = yield* client.prepare(target, {
			snapshotId: snapshot.id,
			directory: configuration.scopeDirectory,
			harness: configuration.harness,
			environmentInputs,
			mcpDirectory: configuration.mcpDirectory
		});
		const envelope = yield* Schema$1.decodeUnknownEffect(LeaseEnvelope)(raw).pipe(Effect.catchTag("SchemaError", (cause) => Effect.fail(new IntegrationError({
			operation: "resource lease credentials",
			reason: "content",
			cause
		}))));
		yield* Effect.addFinalizer(() => connection.poll().pipe(Effect.flatMap((current) => client.release(current, envelope.launch.id, envelope.lifecycleToken))).pipe(Effect.catchTags({
			ConnectionError: (error) => Effect.logWarning(error.message),
			DaemonError: (error) => Effect.logWarning(error.message)
		})));
		const prepared = yield* Schema$1.decodeUnknownEffect(Prepared)(raw).pipe(Effect.catchTag("SchemaError", (cause) => Effect.fail(new IntegrationError({
			operation: "prepared resources",
			reason: "content",
			cause
		}))));
		if (prepared.mcp.some((endpoint) => endpoint.url.origin !== target.url.origin)) return yield* new IntegrationError({
			operation: "MCP endpoint origin",
			reason: "content"
		});
		const skills = yield* Effect.forEach(prepared.content.skills, (tree) => deployer.prepare(tree));
		const endpoints = yield* Effect.forEach(prepared.mcp, (endpoint) => Effect.gen(function* () {
			const digest = yield* crypto.digest("SHA-256", new TextEncoder().encode(endpoint.id)).pipe(Effect.catchTag("PlatformError", (cause) => Effect.fail(new IntegrationError({
				operation: "MCP identity",
				reason: "content",
				cause
			}))));
			return {
				name: `agx-${endpoint.id.replace(/[^a-zA-Z0-9_-]/g, "-").slice(0, 15)}-${Hex.encode(digest).slice(0, 12)}`,
				url: endpoint.url
			};
		}));
		const now = yield* Clock.currentTimeMillis;
		return {
			prepared,
			skills,
			environmentInputs,
			configuration: target.config,
			origin: target.url.origin,
			endpoints,
			instance: target.instance_id,
			renewAt: now + prepared.renewAfterSeconds * 1e3
		};
	})().pipe(Effect.retry({
		times: 2,
		while: (error) => error._tag === "DaemonError" && error.code === Code.Aborted
	}));
	const state = yield* ScopedRef.fromAcquire(acquireCurrent);
	let terminal = null;
	const replace = Effect.fn("lease.replace")(function* () {
		yield* ScopedRef.set(state, acquireCurrent);
		yield* onChange;
		terminal = null;
	});
	const restore = Effect.fn("lease.restore")(function* (target, active) {
		yield* client.attach(target, {
			launch: active.prepared.launch,
			grant: active.prepared.grant,
			lifecycleToken: active.prepared.lifecycleToken,
			environmentInputs: active.environmentInputs,
			mcpDirectory: active.prepared.mcpDirectory
		});
		if ((yield* client.resolve(target, {
			directory: configuration.scopeDirectory,
			harness: configuration.harness
		})).snapshot?.id !== active.prepared.launch.snapshotId || active.origin !== target.url.origin) {
			yield* replace();
			return;
		}
		yield* client.renew(target, active.prepared.launch.id, active.prepared.lifecycleToken);
		active.instance = target.instance_id;
		active.renewAt = (yield* Clock.currentTimeMillis) + active.prepared.renewAfterSeconds * 1e3;
	});
	const synchronize = Effect.fn("lease.synchronize")(function* () {
		const target = yield* connection.poll();
		const active = yield* ScopedRef.get(state);
		if (active.instance !== target.instance_id) {
			yield* restore(target, active);
			return;
		}
		const now = yield* Clock.currentTimeMillis;
		if (now < active.renewAt) return;
		yield* client.renew(target, active.prepared.launch.id, active.prepared.lifecycleToken).pipe(Effect.catchTag("DaemonError", (error) => error.code === Code.NotFound || error.ended === "expired" ? restore(target, active) : Effect.fail(error)));
		active.renewAt = now + active.prepared.renewAfterSeconds * 1e3;
	});
	return {
		current: () => ScopedRef.getUnsafe(state),
		ensure: Effect.suspend(() => terminal === null ? synchronize() : Effect.fail(terminal)).pipe(Effect.catchTag("DaemonError", (error) => error.ended === "revoked" || error.ended === "finished" ? Effect.sync(() => {
			terminal = error;
		}).pipe(Effect.andThen(Effect.fail(error))) : error.code === Code.Unavailable || error.code === Code.Unknown || error.code === Code.DeadlineExceeded ? connection.reconnect(ScopedRef.getUnsafe(state).instance).pipe(Effect.andThen(synchronize())) : Effect.fail(error)), lock.withPermits(1)),
		refresh: replace().pipe(lock.withPermits(1))
	};
});
// ../../node_modules/.pnpm/@effect+platform-node-shared@4.0.6_effect@4.0.6/node_modules/@effect/platform-node-shared/dist/internal/nodeChildProcessSpawner.js
const buildSpawnOptions = (options, base, platform) => {
	const detached = options.detached ?? platform !== "win32";
	return {
		...base,
		detached,
		shell: options.shell,
		windowsHide: options.windowsHide ?? !detached
	};
};
// ../../node_modules/.pnpm/@effect+platform-node-shared@4.0.6_effect@4.0.6/node_modules/@effect/platform-node-shared/dist/internal/utils.js
/** @internal */
const handleErrnoException = (module, method) => (err, [path]) => {
	let reason = "Unknown";
	switch (err.code) {
		case "ENOENT":
			reason = "NotFound";
			break;
		case "EACCES":
		case "EPERM":
			reason = "PermissionDenied";
			break;
		case "EEXIST":
			reason = "AlreadyExists";
			break;
		case "EISDIR":
			reason = "BadResource";
			break;
		case "ENOTDIR":
			reason = "BadResource";
			break;
		case "EBUSY":
			reason = "Busy";
			break;
		case "ELOOP": reason = "BadResource";
	}
	return PlatformError.systemError({
		_tag: reason,
		module,
		method,
		pathOrDescriptor: path,
		syscall: err.syscall,
		cause: err
	});
};
// ../../node_modules/.pnpm/@effect+platform-node-shared@4.0.6_effect@4.0.6/node_modules/@effect/platform-node-shared/dist/internal/stream.js
const destroyStream = (stream, cleanup, onDestroyed) => {
	if (!("closed" in stream)) {
		cleanup();
		onDestroyed?.();
		return;
	}
	const onDone = () => {
		clearImmediate(immediate);
		stream.off("error", onDone);
		stream.off("close", onDone);
		onDestroyed?.();
	};
	const onClosed = () => {
		if (stream.closed) onDone();
		else {
			immediate = setImmediate(onClosed);
			if (!onDestroyed) immediate.unref();
		}
	};
	let immediate = setImmediate(onClosed);
	if (!onDestroyed) immediate.unref();
	stream.once("error", onDone);
	stream.once("close", onDone);
	cleanup();
	if (!stream.closed) stream.destroy();
};
// ../../node_modules/.pnpm/@effect+platform-node-shared@4.0.6_effect@4.0.6/node_modules/@effect/platform-node-shared/dist/NodeSink.js
/**
* Creates a `Sink` that writes chunks to a Node writable stream, respecting
* backpressure and mapping writable errors with `onError`. Unless `endOnDone`
* is `false`, the writable is ended on completion and destroyed on upstream
* failure or interruption.
*
* @stability unstable
* @category constructors
* @since 4.0.0
*/
const fromWritable = (options) => Sink.fromChannel(Channel.mapDone(fromWritableChannel(options), (_) => [_]));
/**
* Creates a `Channel` that pulls chunks from upstream and writes them to a
* Node writable stream, respecting backpressure. Unless `endOnDone` is
* `false`, the writable is ended when upstream is done and destroyed on
* upstream failure or interruption.
*
* @stability unstable
* @category constructors
* @since 4.0.0
*/
const fromWritableChannel = (options) => Channel.fromTransform((pull) => {
	const writable = options.evaluate();
	const write = pullIntoWritable({
		...options,
		writable,
		pull
	});
	return Effect.succeed(options.endOnDone === false ? write : Effect.onError(write, (cause) => Pull.isDoneCause(cause) ? Effect.void : releaseWritable(writable)));
});
const releaseWritable = (writable) => Effect.callback((resume) => {
	const onDestroyed = () => resume(Effect.void);
	if (writable.closed) {
		const cleanup = finished(writable, { readable: false }, () => {
			cleanup();
			onDestroyed();
		});
	} else destroyStream(writable, constVoid, onDestroyed);
});
/**
* Writes Effect chunks into a Node writable stream.
*
* **When to use**
*
* Use to implement custom Node stream adapters that already have an upstream
* pull and need direct control over a writable stream.
*
* **Details**
*
* The loop waits for `drain` when needed, fails on writable errors, and ends
* the writable on upstream completion unless `endOnDone` is `false`.
*
* @stability unstable
* @category converting
* @since 4.0.0
*/
const pullIntoWritable = (options) => Effect.withFiber((fiber) => {
	const dispatcher = fiber.currentDispatcher;
	const completion = Deferred.makeUnsafe();
	const failure = Deferred.makeUnsafe();
	const onError = (error) => Deferred.doneUnsafe(failure, Effect.failSync(() => options.onError(error)));
	let errorEmitted = false;
	const onStreamError = (error) => {
		errorEmitted = true;
		onError(error);
	};
	options.writable.on("error", onStreamError);
	const cleanup = finished(options.writable, { readable: false }, (error) => {
		if (error) onError(error);
		else Deferred.doneUnsafe(completion, Effect.void);
	});
	return options.pull.pipe(Effect.flatMap((chunk) => {
		let i = 0;
		return Effect.callback((resume) => {
			let cancelled = false;
			const onFinish = () => dispatcher.scheduleTask(loop, 0);
			const loop = () => {
				options.writable.off("drain", loop);
				options.writable.off("finish", onFinish);
				for (; i < chunk.length;) {
					if (cancelled) return;
					if (!options.writable.write(chunk[i++], options.encoding, (error) => {
						if (!error) return;
						if (options.writable.closed) onError(error);
						else dispatcher.scheduleTask(() => onError(error), 0);
					})) {
						if (!cancelled) {
							options.writable.once("drain", loop);
							options.writable.once("finish", onFinish);
						}
						return;
					}
				}
				if (!cancelled) resume(Effect.void);
			};
			loop();
			return Effect.sync(() => {
				cancelled = true;
				options.writable.off("drain", loop);
				options.writable.off("finish", onFinish);
			});
		});
	}), Effect.forever({ disableYield: true }), options.endOnDone !== false ? Pull.catchDone((_) => {
		options.writable.end();
		return Effect.andThen(Deferred.await(completion), Cause.done(_));
	}) : identity, Effect.raceFirst(Deferred.await(failure)), Effect.ensuring(Effect.sync(() => {
		cleanup();
		const removeErrorListener = () => {
			options.writable.off("error", onStreamError);
			options.writable.off("error", removeErrorListener);
			options.writable.off("close", removeErrorListener);
		};
		if (options.writable.errored && !options.writable.closed && !errorEmitted) {
			options.writable.once("error", removeErrorListener);
			options.writable.once("close", removeErrorListener);
		} else removeErrorListener();
	})));
});
// ../../node_modules/.pnpm/@effect+platform-node-shared@4.0.6_effect@4.0.6/node_modules/@effect/platform-node-shared/dist/NodeStream.js
/**
* Adapters between Node streams and Effect streams, channels, and readables.
*
* This module is the stream boundary for Node APIs. It wraps `Readable` and
* `Duplex` values as Effect `Stream`s and `Channel`s, pipes Effect streams
* through Node duplex streams, exposes an Effect `Stream` back to Node as a
* `Readable`, and collects readable payloads into strings, array buffers, or
* `Uint8Array`s with optional byte limits.
*
* @stability unstable
* @since 4.0.0
*/
/**
* Converts a Node readable stream into an Effect `Stream`, reading chunks with
* an optional chunk size, mapping stream errors with `onError`, and destroying
* the readable on completion unless `closeOnDone` is `false`.
*
* @stability unstable
* @category constructors
* @since 4.0.0
*/
const fromReadable = (options) => Stream.fromChannel(fromReadableChannel(options));
/**
* Creates a `Channel` that pulls chunks from a Node readable stream, mapping
* errors with `onError` and destroying the readable on completion unless
* `closeOnDone` is `false`.
*
* @stability unstable
* @category constructors
* @since 4.0.0
*/
const fromReadableChannel = (options) => Channel.fromTransform((_, scope) => readableToPullUnsafe({
	scope,
	readable: options.evaluate(),
	onError: options.onError ?? defaultOnError,
	chunkSize: options.chunkSize,
	closeOnDone: options.closeOnDone
}));
const readableToPullUnsafe = (options) => {
	const readable = options.readable;
	const closeOnDone = options.closeOnDone ?? true;
	const exit = options.exit ?? MutableRef.make(void 0);
	const latch = options.latch ?? Latch.makeUnsafe(false);
	function onReadable() {
		latch.openUnsafe();
	}
	readable.on("readable", onReadable);
	const cleanup = finished(readable, {
		readable: true,
		writable: false
	}, (error) => {
		exit.current ??= Exit.fail(error ? options.onError(error) : Cause.Done());
		latch.openUnsafe();
	});
	const pull = Effect.suspend(function loop() {
		let item = options.readable.read(options.chunkSize);
		if (item === null) {
			if (exit.current) return exit.current;
			if (readable.readableEnded) return Effect.fail(Cause.Done());
			latch.closeUnsafe();
			return Effect.flatMap(latch.await, loop);
		}
		const chunk = Arr.of(item);
		while (true) {
			item = options.readable.read(options.chunkSize);
			if (item === null) break;
			chunk.push(item);
		}
		return Effect.succeed(chunk);
	});
	return Effect.as(Scope.addFinalizer(options.scope, Effect.sync(() => {
		readable.off("readable", onReadable);
		if (closeOnDone) destroyStream(readable, cleanup);
		else cleanup();
	})), pull);
};
const defaultOnError = (error) => new Cause.UnknownError(error);
// ../../node_modules/.pnpm/@effect+platform-node-shared@4.0.6_effect@4.0.6/node_modules/@effect/platform-node-shared/dist/NodeChildProcessSpawner.js
const toError = (error) => error instanceof globalThis.Error ? error : new globalThis.Error(String(error));
const toPlatformError = (method, error, command) => {
	const { commands } = flattenCommand(command);
	const commandStr = commands.reduce((acc, curr) => {
		const cmd = `${curr.command} ${curr.args.join(" ")}`;
		return acc.length === 0 ? cmd : `${acc} | ${cmd}`;
	}, "");
	return handleErrnoException("ChildProcess", method)(error, [commandStr]);
};
const processGroupGraceMillis = 1e3;
const processGroupPollIntervalMillis = 10;
const handleInputErrors = (writable) => {
	const onError = () => {};
	writable.on("error", onError);
	writable.once("close", () => writable.off("error", onError));
};
/** The leader has not exited, or a member of its process group still exists. */
const isProcessAlive = (childProcess, exitSignal) => {
	if (!Deferred.isDoneUnsafe(exitSignal)) return true;
	if (globalThis.process.platform === "win32") return false;
	try {
		globalThis.process.kill(-childProcess.pid, 0);
		return true;
	} catch {
		return false;
	}
};
const taskkill = (childProcess, onExit = () => {}) => {
	if (childProcess.exitCode !== null || childProcess.signalCode !== null) {
		onExit(null);
		return;
	}
	NodeChildProcess.execFile("taskkill", [
		"/pid",
		String(childProcess.pid),
		"/T",
		"/F"
	], { windowsHide: true }, onExit);
};
const make$4 = /*#__PURE__*/ Effect.gen(function* () {
	const fs = yield* FileSystem.FileSystem;
	const path = yield* Path$2.Path;
	const resolveWorkingDirectory = Effect.fnUntraced(function* (options) {
		if (Predicate.isUndefined(options.cwd)) return void 0;
		yield* fs.access(options.cwd);
		return path.resolve(options.cwd);
	});
	const resolveEnvironment = (options) => {
		return options.extendEnv ? {
			...globalThis.process.env,
			...options.env
		} : options.env;
	};
	const inputToStdioOption = (input) => Stream.isStream(input) ? "pipe" : input;
	const outputToStdioOption = (input) => Sink.isSink(input) ? "pipe" : input;
	const resolveStdinOption = (options) => {
		const defaultConfig = {
			stream: "pipe",
			encoding: "utf-8",
			endOnDone: true
		};
		if (Predicate.isUndefined(options.stdin)) return defaultConfig;
		if (typeof options.stdin === "string") return {
			...defaultConfig,
			stream: options.stdin
		};
		if (Stream.isStream(options.stdin)) return {
			...defaultConfig,
			stream: options.stdin
		};
		return {
			stream: options.stdin.stream,
			encoding: options.stdin.encoding ?? defaultConfig.encoding,
			endOnDone: options.stdin.endOnDone ?? defaultConfig.endOnDone
		};
	};
	const resolveOutputOption = (options, streamName) => {
		const option = options[streamName];
		if (Predicate.isUndefined(option)) return { stream: "pipe" };
		if (typeof option === "string") return { stream: option };
		if (Sink.isSink(option)) return { stream: option };
		return { stream: option.stream };
	};
	const resolveAdditionalFds = (options) => {
		if (Predicate.isUndefined(options.additionalFds)) return [];
		const result = [];
		for (const [name, config] of Object.entries(options.additionalFds)) {
			const fd = ChildProcess.parseFdName(name);
			if (Predicate.isNotUndefined(fd)) result.push({
				fd,
				config
			});
		}
		return result.sort((a, b) => a.fd - b.fd);
	};
	const buildStdioArray = (stdinConfig, stdoutConfig, stderrConfig, additionalFds) => {
		const stdio = [
			inputToStdioOption(stdinConfig.stream),
			outputToStdioOption(stdoutConfig.stream),
			outputToStdioOption(stderrConfig.stream)
		];
		if (additionalFds.length === 0) return stdio;
		const maxFd = additionalFds.reduce((max, { fd }) => Math.max(max, fd), 2);
		for (let i = 3; i <= maxFd; i++) stdio[i] = "ignore";
		for (const { fd } of additionalFds) stdio[fd] = "pipe";
		return stdio;
	};
	const setupAdditionalFds = Effect.fnUntraced(function* (command, childProcess, additionalFds) {
		if (additionalFds.length === 0) return {
			getInputFd: () => Sink.drain,
			getOutputFd: () => Stream.empty
		};
		const inputSinks = /* @__PURE__ */ new Map();
		const outputStreams = /* @__PURE__ */ new Map();
		for (const { config, fd } of additionalFds) {
			const nodeStream = childProcess.stdio[fd];
			switch (config.type) {
				case "input": {
					let sink = Sink.drain;
					if (nodeStream && "write" in nodeStream) {
						handleInputErrors(nodeStream);
						sink = fromWritable({
							evaluate: () => nodeStream,
							onError: (error) => toPlatformError(`fromWritable(fd${fd})`, toError(error), command)
						});
					}
					if (config.stream) yield* Effect.forkScoped(Stream.run(config.stream, sink));
					inputSinks.set(fd, sink);
					break;
				}
				case "output": {
					let stream = Stream.empty;
					if (nodeStream && "read" in nodeStream) {
						const passThrough = new PassThrough();
						nodeStream.on("error", (error) => passThrough.destroy(error));
						nodeStream.pipe(passThrough);
						stream = fromReadable({
							evaluate: () => passThrough,
							onError: (error) => toPlatformError(`fromReadable(fd${fd})`, toError(error), command)
						});
					}
					if (config.sink) stream = Stream.transduce(stream, config.sink);
					outputStreams.set(fd, stream);
					break;
				}
			}
		}
		return {
			getInputFd: (fd) => inputSinks.get(fd) ?? Sink.drain,
			getOutputFd: (fd) => outputStreams.get(fd) ?? Stream.empty
		};
	});
	const setupChildStdin = (command, childProcess, config) => Effect.suspend(() => {
		let sink = Sink.drain;
		if (Predicate.isNotNull(childProcess.stdin)) {
			handleInputErrors(childProcess.stdin);
			sink = fromWritable({
				evaluate: () => childProcess.stdin,
				onError: (error) => toPlatformError("fromWritable(stdin)", toError(error), command),
				endOnDone: config.endOnDone,
				encoding: config.encoding
			});
		}
		if (Stream.isStream(config.stream)) return Effect.as(Effect.forkScoped(Stream.run(config.stream, sink)), sink);
		return Effect.succeed(sink);
	});
	const setupChildOutputStreams = (command, childProcess, stdoutConfig, stderrConfig) => {
		let stdout = childProcess.stdout ? (() => {
			const passThrough = new PassThrough();
			childProcess.stdout.on("error", (error) => passThrough.destroy(error));
			childProcess.stdout.pipe(passThrough);
			return fromReadable({
				evaluate: () => passThrough,
				onError: (error) => toPlatformError("fromReadable(stdout)", toError(error), command)
			});
		})() : Stream.empty;
		let stderr = childProcess.stderr ? (() => {
			const passThrough = new PassThrough();
			childProcess.stderr.on("error", (error) => passThrough.destroy(error));
			childProcess.stderr.pipe(passThrough);
			return fromReadable({
				evaluate: () => passThrough,
				onError: (error) => toPlatformError("fromReadable(stderr)", toError(error), command)
			});
		})() : Stream.empty;
		if (Sink.isSink(stdoutConfig.stream)) stdout = Stream.transduce(stdout, stdoutConfig.stream);
		if (Sink.isSink(stderrConfig.stream)) stderr = Stream.transduce(stderr, stderrConfig.stream);
		const all = Stream.merge(stdout, stderr);
		return {
			stdout,
			stderr,
			all
		};
	};
	const spawn = (command, spawnOptions) => Effect.callback((resume) => {
		const deferred = Deferred.makeUnsafe();
		const handle = NodeChildProcess.spawn(command.command, command.args, spawnOptions);
		handle.on("error", (error) => {
			resume(Effect.fail(toPlatformError("spawn", error, command)));
		});
		handle.on("exit", (...args) => {
			Deferred.doneUnsafe(deferred, Exit.succeed(args));
		});
		handle.on("spawn", () => {
			resume(Effect.succeed([handle, deferred]));
		});
		return Effect.sync(() => {
			handle.kill("SIGTERM");
		});
	});
	const killProcessGroup = (command, childProcess, signal) => {
		if (globalThis.process.platform === "win32") return Effect.callback((resume) => {
			taskkill(childProcess, (error) => {
				if (error) resume(Effect.fail(toPlatformError("kill", toError(error), command)));
				else resume(Effect.void);
			});
		});
		return Effect.try({
			try: () => {
				globalThis.process.kill(-childProcess.pid, signal);
			},
			catch: (error) => toPlatformError("kill", toError(error), command)
		});
	};
	const killProcessGroupOnExit = (childProcess, signal) => {
		if (globalThis.process.platform === "win32") {
			taskkill(childProcess);
			return;
		}
		try {
			globalThis.process.kill(-childProcess.pid, signal);
		} catch {}
	};
	const killProcess = (command, childProcess, signal) => Effect.suspend(() => {
		if (!childProcess.kill(signal)) {
			const error = new globalThis.Error("Failed to kill child process");
			return Effect.fail(toPlatformError("kill", error, command));
		}
		return Effect.void;
	});
	/** Waits for the leader and its process group, bounded by native time. */
	const awaitProcessExit = (childProcess, exitSignal, timeoutMillis) => Effect.callback((resume) => {
		const deadline = Date.now() + timeoutMillis;
		let timer;
		const stop = () => {
			clearTimeout(timer);
			childProcess.removeListener("exit", poll);
		};
		const poll = () => {
			clearTimeout(timer);
			if (Date.now() >= deadline || !isProcessAlive(childProcess, exitSignal)) {
				stop();
				resume(Effect.void);
				return;
			}
			timer = setTimeout(poll, processGroupPollIntervalMillis);
		};
		childProcess.on("exit", poll);
		poll();
		return Effect.sync(stop);
	});
	const terminateProcessGroup = Effect.fnUntraced(function* (command, childProcess, exitSignal, options) {
		const signalGroup = (signal) => killProcessGroup(command, childProcess, signal).pipe(Effect.catch(() => killProcess(command, childProcess, signal)));
		yield* signalGroup(options?.killSignal ?? "SIGTERM");
		if (Predicate.isUndefined(options?.forceKillAfter)) {
			yield* Deferred.await(exitSignal);
			yield* awaitProcessExit(childProcess, exitSignal, processGroupGraceMillis);
		} else {
			yield* awaitProcessExit(childProcess, exitSignal, Duration.toMillis(options.forceKillAfter));
			if (isProcessAlive(childProcess, exitSignal)) {
				yield* signalGroup("SIGKILL");
				yield* awaitProcessExit(childProcess, exitSignal, processGroupGraceMillis);
			}
		}
		yield* Deferred.await(exitSignal);
	});
	/**
	* Get the appropriate source stream from a process handle based on the
	* `from` pipe option.
	*/
	const getSourceStream = (handle, from) => {
		const fromOption = from ?? "stdout";
		switch (fromOption) {
			case "stdout": return handle.stdout;
			case "stderr": return handle.stderr;
			case "all": return handle.all;
			default: {
				const fd = ChildProcess.parseFdName(fromOption);
				if (Predicate.isNotUndefined(fd)) return handle.getOutputFd(fd);
				return handle.stdout;
			}
		}
	};
	const spawnCommand = Effect.fnUntraced(function* (cmd) {
		switch (cmd._tag) {
			case "StandardCommand": {
				const stdinConfig = resolveStdinOption(cmd.options);
				const stdoutConfig = resolveOutputOption(cmd.options, "stdout");
				const stderrConfig = resolveOutputOption(cmd.options, "stderr");
				const resolvedAdditionalFds = resolveAdditionalFds(cmd.options);
				let isReferenced = true;
				const cwd = yield* resolveWorkingDirectory(cmd.options);
				const env = resolveEnvironment(cmd.options);
				const stdio = buildStdioArray(stdinConfig, stdoutConfig, stderrConfig, resolvedAdditionalFds);
				const [childProcess, exitSignal] = yield* Effect.acquireRelease(spawn(cmd, buildSpawnOptions(cmd.options, {
					cwd,
					env,
					stdio
				}, process.platform)), Effect.fnUntraced(function* ([childProcess, exitSignal]) {
					if (yield* Deferred.isDone(exitSignal)) {
						const [code] = yield* Deferred.await(exitSignal);
						if (code !== 0 && Predicate.isNotNull(code)) yield* Effect.ignore(terminateProcessGroup(cmd, childProcess, exitSignal, cmd.options));
						else if (isReferenced && process.platform !== "win32" && cmd.options.detached !== false) yield* Effect.ignore(terminateProcessGroup(cmd, childProcess, exitSignal, cmd.options));
						return;
					}
					if (!isReferenced) return;
					yield* Effect.ignore(terminateProcessGroup(cmd, childProcess, exitSignal, cmd.options));
				}));
				const pid = ProcessId(childProcess.pid);
				childProcess.on("exit", (code) => {
					if (code !== 0 && Predicate.isNotNull(code)) killProcessGroupOnExit(childProcess, cmd.options.killSignal ?? "SIGTERM");
				});
				const reref = Effect.sync(() => {
					if (!isReferenced) {
						childProcess.ref();
						isReferenced = true;
					}
				});
				const unref = Effect.sync(() => {
					if (isReferenced) {
						childProcess.unref();
						isReferenced = false;
					}
					return reref;
				});
				const stdin = yield* setupChildStdin(cmd, childProcess, stdinConfig);
				const { all, stderr, stdout } = setupChildOutputStreams(cmd, childProcess, stdoutConfig, stderrConfig);
				const { getInputFd, getOutputFd } = yield* setupAdditionalFds(cmd, childProcess, resolvedAdditionalFds);
				const isRunning = Effect.map(Deferred.isDone(exitSignal), (done) => !done);
				const exitCode = Effect.flatMap(Deferred.await(exitSignal), ([code, signal]) => {
					if (Predicate.isNotNull(code)) return Effect.succeed(ExitCode(code));
					const error = new globalThis.Error(`Process interrupted due to receipt of signal: '${signal}'`);
					return Effect.fail(toPlatformError("exitCode", error, cmd));
				});
				const kill = (options) => terminateProcessGroup(cmd, childProcess, exitSignal, options);
				return makeHandle({
					pid,
					exitCode,
					isRunning,
					kill,
					stdin,
					stdout,
					stderr,
					all,
					getInputFd,
					getOutputFd,
					unref
				});
			}
			case "PipedCommand": {
				const { commands, pipeOptions } = flattenCommand(cmd);
				const [root, ...pipeline] = commands;
				const handles = [yield* spawnCommand(root)];
				for (let i = 0; i < pipeline.length; i++) {
					const command = pipeline[i];
					const options = pipeOptions[i] ?? {};
					const stdinConfig = resolveStdinOption(command.options);
					const sourceStream = Stream.unwrap(Effect.succeed(getSourceStream(handles[handles.length - 1], options.from)));
					const toOption = options.to ?? "stdin";
					if (toOption === "stdin") handles.push(yield* spawnCommand(ChildProcess.make(command.command, command.args, {
						...command.options,
						stdin: {
							...stdinConfig,
							stream: sourceStream
						}
					})));
					else {
						const fd = ChildProcess.parseFdName(toOption);
						if (Predicate.isNotUndefined(fd)) {
							const fdName = ChildProcess.fdName(fd);
							const existingFds = command.options.additionalFds ?? {};
							handles.push(yield* spawnCommand(ChildProcess.make(command.command, command.args, {
								...command.options,
								additionalFds: {
									...existingFds,
									[fdName]: {
										type: "input",
										stream: sourceStream
									}
								}
							})));
						} else handles.push(yield* spawnCommand(ChildProcess.make(command.command, command.args, {
							...command.options,
							stdin: {
								...stdinConfig,
								stream: sourceStream
							}
						})));
					}
				}
				const handle = handles[handles.length - 1];
				const kill = (options) => Effect.forEach([...handles].reverse(), (handle) => Effect.ignore(handle.kill(options)), { discard: true });
				const unref = Effect.gen(function* () {
					const rerefs = [];
					for (const handle of handles) rerefs.push(yield* handle.unref);
					return Effect.forEach([...rerefs].reverse(), (reref) => reref, { discard: true });
				});
				return makeHandle({
					pid: handle.pid,
					exitCode: handle.exitCode,
					isRunning: handle.isRunning,
					kill,
					stdin: handles[0].stdin,
					stdout: handle.stdout,
					stderr: handle.stderr,
					all: handle.all,
					getInputFd: handle.getInputFd,
					getOutputFd: handle.getOutputFd,
					unref
				});
			}
		}
	});
	return make$1(spawnCommand);
});
/**
* Layer that provides the `NodeChildProcessSpawner` implementation.
*
* @stability unstable
* @category layers
* @since 4.0.0
*/
const layer$11 = /*#__PURE__*/ Layer.effect(ChildProcessSpawner, make$4);
/**
* Flattens a `Command` into an array of `StandardCommand`s along with pipe
* options for each connection.
*
* @stability unstable
* @category transforming
* @since 4.0.0
*/
const flattenCommand = (command) => {
	const commands = [];
	const pipeOptions = [];
	const flatten = (cmd) => {
		switch (cmd._tag) {
			case "StandardCommand":
				commands.push(cmd);
				break;
			case "PipedCommand":
				flatten(cmd.left);
				pipeOptions.push(cmd.options);
				flatten(cmd.right);
		}
	};
	flatten(command);
	if (commands.length === 0) throw new Error("flattenCommand produced empty commands array");
	const [first, ...rest] = commands;
	return {
		commands: [first, ...rest],
		pipeOptions
	};
};
// ../../node_modules/.pnpm/@effect+platform-node-shared@4.0.6_effect@4.0.6/node_modules/@effect/platform-node-shared/dist/NodeCrypto.js
/**
* Node-compatible implementation of Effect's `Crypto` service.
*
* This module builds the service from `node:crypto`, using `randomBytes` for
* random data and `createHash` for supported digest algorithms. It exports
* `make` as the concrete service value and `layer` for providing it through
* Effect context.
*
* @stability unstable
* @since 1.0.0
*/
const toHashAlgorithm = (algorithm) => {
	switch (algorithm) {
		case "SHA-1": return "sha1";
		case "SHA-256": return "sha256";
		case "SHA-384": return "sha384";
		case "SHA-512": return "sha512";
	}
};
const digest = (algorithm, data) => Effect.try({
	try: () => Uint8Array.from(NodeCrypto.createHash(toHashAlgorithm(algorithm)).update(data).digest()),
	catch: (cause) => PlatformError.systemError({
		module: "Crypto",
		method: "digest",
		_tag: "Unknown",
		description: "Could not compute digest",
		cause
	})
});
/**
* The default Node.js Crypto service implementation.
*
* @stability unstable
* @category constructors
* @since 1.0.0
*/
const make$3 = /*#__PURE__*/ Crypto.make({
	randomBytes: NodeCrypto.randomBytes,
	digest
});
// ../../node_modules/.pnpm/@effect+platform-node@4.0.6_effect@4.0.6/node_modules/@effect/platform-node/dist/NodeCrypto.js
/**
* The `NodeCrypto` module provides the Node.js `Crypto` service layer for
* Effect programs. Provide {@link layer} at the edge of a Node application,
* CLI, script, or test to satisfy `effect/Crypto` with Node's `node:crypto`
* implementation for secure random bytes, UUID generation, random values, and
* SHA digest operations.
*
* This module is the public Node adapter around the shared Node-compatible
* implementation. Digest failures are reported as platform errors, and SHA-1
* remains available only for interoperability with existing protocols.
*
* @stability unstable
* @since 1.0.0
*/
/**
* Layer that provides the Node.js Crypto service implementation.
*
* @stability unstable
* @category layers
* @since 1.0.0
*/
const layer$9 = /* @__PURE__ */ Layer.succeed(Crypto.Crypto, make$3);
// ../../node_modules/.pnpm/@effect+platform-node-shared@4.0.6_effect@4.0.6/node_modules/@effect/platform-node-shared/dist/NodeFileSystem.js
/**
* Shared Node-compatible implementation of Effect's `FileSystem` service.
*
* This module adapts Node's `node:fs`, `node:os`, and `node:path` APIs into a
* `FileSystem` layer for Effect programs running on Node-compatible runtimes.
* Platform packages use it to provide file and directory I/O, permissions,
* links, metadata, temporary files and directories, and file watching through
* the shared `FileSystem` service.
*
* @stability unstable
* @since 4.0.0
*/
const handleBadArgument = (method) => (err) => PlatformError.badArgument({
	module: "FileSystem",
	method,
	description: err.message ?? String(err)
});
const bigintToNumber = (value, field) => {
	const number = Number(value);
	if (!Number.isSafeInteger(number)) throw new RangeError(`${field} exceeds the safe integer range: ${value}`);
	return number;
};
const bigintToNumberOption = (value) => Option$1.flatMap(Option$1.fromNullishOr(value), BI.toNumber);
const toUint64 = (value) => BigInt.asUintN(64, value);
const toUint64Option = (value) => Option$1.map(Option$1.fromNullishOr(value), toUint64);
const positionToNumber = (position, method) => Effect.try({
	try: () => bigintToNumber(position, "position"),
	catch: handleBadArgument(method)
});
const access = /*#__PURE__*/ (() => {
	const nodeAccess = /*#__PURE__*/ effectify(NFS.access, /*#__PURE__*/ handleErrnoException("FileSystem", "access"), /*#__PURE__*/ handleBadArgument("access"));
	return (path, options) => {
		let mode = NFS.constants.F_OK;
		if (options?.readable) mode |= NFS.constants.R_OK;
		if (options?.writable) mode |= NFS.constants.W_OK;
		return nodeAccess(path, mode);
	};
})();
const copy = /*#__PURE__*/ (() => {
	const nodeCp = /*#__PURE__*/ effectify(NFS.cp, /*#__PURE__*/ handleErrnoException("FileSystem", "copy"), /*#__PURE__*/ handleBadArgument("copy"));
	return (fromPath, toPath, options) => nodeCp(fromPath, toPath, {
		force: options?.overwrite ?? false,
		preserveTimestamps: options?.preserveTimestamps ?? false,
		recursive: true
	});
})();
const copyFile = /*#__PURE__*/ (() => {
	const nodeCopyFile = /*#__PURE__*/ effectify(NFS.copyFile, /*#__PURE__*/ handleErrnoException("FileSystem", "copyFile"), /*#__PURE__*/ handleBadArgument("copyFile"));
	return (fromPath, toPath) => nodeCopyFile(fromPath, toPath);
})();
const chmod = /*#__PURE__*/ (() => {
	const nodeChmod = /*#__PURE__*/ effectify(NFS.chmod, /*#__PURE__*/ handleErrnoException("FileSystem", "chmod"), /*#__PURE__*/ handleBadArgument("chmod"));
	return (path, mode) => nodeChmod(path, mode);
})();
const chown = /*#__PURE__*/ (() => {
	const nodeChown = /*#__PURE__*/ effectify(NFS.chown, /*#__PURE__*/ handleErrnoException("FileSystem", "chown"), /*#__PURE__*/ handleBadArgument("chown"));
	return (path, uid, gid) => nodeChown(path, uid, gid);
})();
const glob = /*#__PURE__*/ (() => {
	const nodeGlob = /*#__PURE__*/ effectify(NFS.glob, /*#__PURE__*/ handleErrnoException("FileSystem", "glob"), /*#__PURE__*/ handleBadArgument("glob"));
	return (pattern, options) => nodeGlob(pattern, {
		cwd: options?.root,
		exclude: options?.exclude
	});
})();
const link = /*#__PURE__*/ (() => {
	const nodeLink = /*#__PURE__*/ effectify(NFS.link, /*#__PURE__*/ handleErrnoException("FileSystem", "link"), /*#__PURE__*/ handleBadArgument("link"));
	return (existingPath, newPath) => nodeLink(existingPath, newPath);
})();
const makeDirectory = /*#__PURE__*/ (() => {
	const nodeMkdir = /*#__PURE__*/ effectify(NFS.mkdir, /*#__PURE__*/ handleErrnoException("FileSystem", "makeDirectory"), /*#__PURE__*/ handleBadArgument("makeDirectory"));
	return (path, options) => nodeMkdir(path, {
		recursive: options?.recursive ?? false,
		mode: options?.mode
	});
})();
const makeTempDirectoryFactory = (method) => {
	const nodeMkdtemp = effectify(NFS.mkdtemp, handleErrnoException("FileSystem", method), handleBadArgument(method));
	return (options) => Effect.suspend(() => {
		const prefix = options?.prefix ?? "";
		const directory = typeof options?.directory === "string" ? Path$1.join(options.directory, ".") : OS.tmpdir();
		return nodeMkdtemp(prefix ? Path$1.join(directory, prefix) : directory + "/");
	});
};
const makeTempDirectory = /*#__PURE__*/ makeTempDirectoryFactory("makeTempDirectory");
const removeFactory = (method) => {
	const nodeRm = effectify(NFS.rm, handleErrnoException("FileSystem", method), handleBadArgument(method));
	return (path, options) => nodeRm(path, {
		recursive: options?.recursive ?? false,
		force: options?.force ?? false
	});
};
const remove = /*#__PURE__*/ removeFactory("remove");
const makeTempDirectoryScoped = /*#__PURE__*/ (() => {
	const makeDirectory = /*#__PURE__*/ makeTempDirectoryFactory("makeTempDirectoryScoped");
	const removeDirectory = /*#__PURE__*/ removeFactory("makeTempDirectoryScoped");
	return (options) => Effect.acquireRelease(makeDirectory(options), (directory) => Effect.orDie(removeDirectory(directory, { recursive: true })));
})();
const openFlagBits = (flag) => {
	const { O_APPEND, O_CREAT, O_EXCL, O_RDONLY, O_RDWR, O_TRUNC, O_WRONLY } = NFS.constants;
	switch (flag) {
		case "r": return O_RDONLY;
		case "r+": return O_RDWR;
		case "w": return O_TRUNC | O_CREAT | O_WRONLY;
		case "wx": return O_TRUNC | O_CREAT | O_WRONLY | O_EXCL;
		case "w+": return O_TRUNC | O_CREAT | O_RDWR;
		case "wx+": return O_TRUNC | O_CREAT | O_RDWR | O_EXCL;
		case "a": return O_APPEND | O_CREAT | O_WRONLY;
		case "ax": return O_APPEND | O_CREAT | O_WRONLY | O_EXCL;
		case "a+": return O_APPEND | O_CREAT | O_RDWR;
		case "ax+": return O_APPEND | O_CREAT | O_RDWR | O_EXCL;
	}
};
const noFollowFlags = (method, flag) => Effect.suspend(() => {
	const { O_NOFOLLOW } = NFS.constants;
	if (process.platform === "win32" || typeof O_NOFOLLOW !== "number" || O_NOFOLLOW === 0) return Effect.fail(PlatformError.badArgument({
		module: "FileSystem",
		method,
		description: "noFollow is not supported on this platform"
	}));
	return Effect.succeed(openFlagBits(flag) | O_NOFOLLOW);
});
const openFactory = (method) => {
	const nodeOpen = effectify(NFS.open, handleErrnoException("FileSystem", method), handleBadArgument(method));
	const nodeClose = effectify(NFS.close, handleErrnoException("FileSystem", method), handleBadArgument(method));
	return (path, options) => pipe(Effect.acquireRelease(options?.noFollow === true ? Effect.flatMap(noFollowFlags(method, options.flag ?? "r"), (flags) => nodeOpen(path, flags, options.mode)) : nodeOpen(path, options?.flag ?? "r", options?.mode), (fd) => Effect.orDie(nodeClose(fd))), Effect.map((fd) => makeFile(fd, options?.flag?.startsWith("a") ?? false)));
};
const open = /*#__PURE__*/ openFactory("open");
const negativePosition = (method) => PlatformError.badArgument({
	module: "FileSystem",
	method,
	description: "Cannot read before the start of the file"
});
const makeFile = /*#__PURE__*/ (() => {
	const nodeReadFactory = (method) => effectify(NFS.read, handleErrnoException("FileSystem", method), handleBadArgument(method));
	const nodeRead = /*#__PURE__*/ nodeReadFactory("read");
	const nodeReadAlloc = /*#__PURE__*/ nodeReadFactory("readAlloc");
	const nodeStat = /*#__PURE__*/ effectify(NFS.fstat, /*#__PURE__*/ handleErrnoException("FileSystem", "stat"), /*#__PURE__*/ handleBadArgument("stat"));
	const nodeTruncate = /*#__PURE__*/ effectify(NFS.ftruncate, /*#__PURE__*/ handleErrnoException("FileSystem", "truncate"), /*#__PURE__*/ handleBadArgument("truncate"));
	const nodeSync = /*#__PURE__*/ effectify(NFS.fsync, /*#__PURE__*/ handleErrnoException("FileSystem", "sync"), /*#__PURE__*/ handleBadArgument("sync"));
	const nodeWriteFactory = (method) => effectify(NFS.write, handleErrnoException("FileSystem", method), handleBadArgument(method));
	const nodeWrite = /*#__PURE__*/ nodeWriteFactory("write");
	const nodeWriteAll = /*#__PURE__*/ nodeWriteFactory("writeAll");
	class FileImpl {
		[FileSystem.FileTypeId];
		fd;
		append;
		position = /*#__PURE__*/ BigInt(0);
		constructor(fd, append) {
			this[FileSystem.FileTypeId] = FileSystem.FileTypeId;
			this.fd = fd;
			this.append = append;
		}
		get stat() {
			return Effect.flatMap(nodeStat(this.fd, { bigint: true }), makeFileInfo);
		}
		get sync() {
			return nodeSync(this.fd);
		}
		seek(offset, from) {
			return Effect.suspend(() => {
				const position = from === "start" ? offset : this.position + offset;
				if (position < BigInt(0)) return Effect.fail(PlatformError.badArgument({
					module: "FileSystem",
					method: "seek",
					description: "Cannot seek before the start of the file"
				}));
				this.position = position;
				return Effect.succeed(position);
			});
		}
		read(buffer, options) {
			return Effect.suspend(() => {
				const explicit = options?.position;
				if (explicit !== void 0) return explicit < BigInt(0) ? Effect.fail(negativePosition("read")) : nodeRead(this.fd, {
					buffer,
					position: explicit
				});
				const position = this.position;
				return Effect.map(nodeRead(this.fd, {
					buffer,
					position
				}), (bytesRead) => {
					this.position = position + BigInt(bytesRead);
					return bytesRead;
				});
			});
		}
		readAlloc(size, options) {
			return Effect.suspend(() => {
				try {
					if (!Number.isInteger(size) || size < 0) throw new RangeError("size must be a non-negative integer");
					const explicit = options?.position;
					if (explicit !== void 0 && explicit < BigInt(0)) return Effect.fail(negativePosition("readAlloc"));
					const buffer = Buffer.allocUnsafeSlow(size);
					const position = explicit ?? this.position;
					return Effect.map(nodeReadAlloc(this.fd, {
						buffer,
						position
					}), (bytesRead) => {
						if (bytesRead === 0) return Option$1.none();
						if (explicit === void 0) this.position = position + BigInt(bytesRead);
						if (bytesRead === size) return Option$1.some(buffer);
						const dst = Buffer.allocUnsafeSlow(bytesRead);
						buffer.copy(dst, 0, 0, bytesRead);
						return Option$1.some(dst);
					});
				} catch (cause) {
					return Effect.fail(handleBadArgument("readAlloc")(cause));
				}
			});
		}
		truncate(length) {
			return Effect.map(nodeTruncate(this.fd, length || void 0), () => {
				if (!this.append) {
					const len = BigInt(length ?? 0);
					if (this.position > len) this.position = len;
				}
			});
		}
		write(buffer) {
			return Effect.suspend(() => {
				const position = this.position;
				return Effect.flatMap(this.append ? Effect.succeed(void 0) : positionToNumber(position, "write"), (nodePosition) => Effect.map(nodeWrite(this.fd, buffer, void 0, void 0, nodePosition), (bytesWritten) => {
					if (!this.append) this.position = position + BigInt(bytesWritten);
					return bytesWritten;
				}));
			});
		}
		writeAllChunk(buffer) {
			return Effect.suspend(() => {
				const position = this.position;
				return Effect.flatMap(this.append ? Effect.succeed(void 0) : positionToNumber(position, "writeAll"), (nodePosition) => Effect.flatMap(nodeWriteAll(this.fd, buffer, void 0, void 0, nodePosition), (bytesWritten) => {
					if (bytesWritten === 0) return Effect.fail(PlatformError.systemError({
						module: "FileSystem",
						method: "writeAll",
						_tag: "WriteZero",
						pathOrDescriptor: this.fd,
						description: "write returned 0 bytes written"
					}));
					if (!this.append) this.position = position + BigInt(bytesWritten);
					return bytesWritten < buffer.length ? this.writeAllChunk(buffer.subarray(bytesWritten)) : Effect.void;
				}));
			});
		}
		writeAll(buffer) {
			return buffer.length === 0 ? Effect.void : this.writeAllChunk(buffer);
		}
	}
	return (fd, append) => new FileImpl(fd, append);
})();
const makeTempFileFactory = (method) => {
	const makeDirectory = makeTempDirectoryFactory(method);
	return Effect.fnUntraced(function* (options) {
		const directory = yield* makeDirectory(options);
		const random = NodeCrypto.randomBytes(6).toString("hex");
		const name = Path$1.join(directory, options?.suffix ? `${random}${options.suffix}` : random);
		yield* writeFile(name, /* @__PURE__ */ new Uint8Array(0));
		return name;
	});
};
const makeTempFile = /*#__PURE__*/ makeTempFileFactory("makeTempFile");
const makeTempFileScoped = /*#__PURE__*/ (() => {
	const makeFile = /*#__PURE__*/ makeTempFileFactory("makeTempFileScoped");
	const removeDirectory = /*#__PURE__*/ removeFactory("makeTempFileScoped");
	return (options) => Effect.acquireRelease(makeFile(options), (file) => Effect.orDie(removeDirectory(Path$1.dirname(file), { recursive: true })));
})();
const readDirectory = (path, options) => Effect.tryPromise({
	try: () => NFS.promises.readdir(path, options),
	catch: (err) => handleErrnoException("FileSystem", "readDirectory")(err, [path])
});
const readFile = (path) => Effect.callback((resume, signal) => {
	try {
		NFS.readFile(path, { signal }, (err, data) => {
			if (err) resume(Effect.fail(handleErrnoException("FileSystem", "readFile")(err, [path])));
			else resume(Effect.succeed(data));
		});
	} catch (err) {
		resume(Effect.fail(handleBadArgument("readFile")(err)));
	}
});
const readLink = /*#__PURE__*/ (() => {
	const nodeReadLink = /*#__PURE__*/ effectify(NFS.readlink, /*#__PURE__*/ handleErrnoException("FileSystem", "readLink"), /*#__PURE__*/ handleBadArgument("readLink"));
	return (path) => nodeReadLink(path);
})();
const realPath = /*#__PURE__*/ (() => {
	const nodeRealPath = /*#__PURE__*/ effectify(process.platform === "win32" ? NFS.realpath.native : NFS.realpath, /*#__PURE__*/ handleErrnoException("FileSystem", "realPath"), /*#__PURE__*/ handleBadArgument("realPath"));
	return (path) => nodeRealPath(path);
})();
const rename = /*#__PURE__*/ (() => {
	const nodeRename = /*#__PURE__*/ effectify(NFS.rename, /*#__PURE__*/ handleErrnoException("FileSystem", "rename"), /*#__PURE__*/ handleBadArgument("rename"));
	return (oldPath, newPath) => nodeRename(oldPath, newPath);
})();
const makeFileInfo = (stat, method = "stat") => Effect.try({
	try: () => ({
		type: stat.isFile() ? "File" : stat.isDirectory() ? "Directory" : stat.isSymbolicLink() ? "SymbolicLink" : stat.isBlockDevice() ? "BlockDevice" : stat.isCharacterDevice() ? "CharacterDevice" : stat.isFIFO() ? "FIFO" : stat.isSocket() ? "Socket" : "Unknown",
		mtime: Option$1.fromNullishOr(stat.mtime),
		atime: Option$1.fromNullishOr(stat.atime),
		birthtime: Option$1.fromNullishOr(stat.birthtime),
		dev: toUint64(stat.dev),
		rdev: toUint64Option(stat.rdev),
		ino: toUint64Option(stat.ino),
		mode: bigintToNumber(stat.mode, "mode"),
		nlink: toUint64Option(stat.nlink),
		uid: bigintToNumberOption(stat.uid),
		gid: bigintToNumberOption(stat.gid),
		size: ByteSize.bytes(stat.size),
		blksize: stat.blksize !== void 0 ? Option$1.some(ByteSize.bytes(stat.blksize)) : Option$1.none(),
		blocks: Option$1.fromNullishOr(stat.blocks)
	}),
	catch: handleBadArgument(method)
});
const statFactory = (method, nodeStatFn) => {
	const nodeStat = effectify(nodeStatFn, handleErrnoException("FileSystem", method), handleBadArgument(method));
	return (path) => Effect.flatMap(nodeStat(path, { bigint: true }), (stat) => makeFileInfo(stat, method));
};
const stat = /*#__PURE__*/ statFactory("stat", NFS.stat);
const lstat = /*#__PURE__*/ statFactory("lstat", NFS.lstat);
const symlink = /*#__PURE__*/ (() => {
	const nodeSymlink = /*#__PURE__*/ effectify(NFS.symlink, /*#__PURE__*/ handleErrnoException("FileSystem", "symlink"), /*#__PURE__*/ handleBadArgument("symlink"));
	return (target, path) => nodeSymlink(target, path);
})();
const truncate = /*#__PURE__*/ (() => {
	const nodeTruncate = /*#__PURE__*/ effectify(NFS.truncate, /*#__PURE__*/ handleErrnoException("FileSystem", "truncate"), /*#__PURE__*/ handleBadArgument("truncate"));
	return (path, length) => nodeTruncate(path, length);
})();
const utimes = /*#__PURE__*/ (() => {
	const nodeUtimes = /*#__PURE__*/ effectify(NFS.utimes, /*#__PURE__*/ handleErrnoException("FileSystem", "utime"), /*#__PURE__*/ handleBadArgument("utime"));
	return (path, atime, mtime) => nodeUtimes(path, atime, mtime);
})();
const watchNode = (path, info, options) => Stream.callback((queue) => Effect.acquireRelease(Effect.sync(() => {
	const directory = info.type === "Directory" ? path : Path$1.dirname(path);
	const watcher = NFS.watch(path, { recursive: options?.recursive ?? false }, (event, path) => {
		if (!path) return;
		switch (event) {
			case "rename":
				Effect.runFork(Effect.matchEffect(stat(Path$1.resolve(directory, path)), {
					onSuccess: (_) => Queue.offer(queue, {
						_tag: "Create",
						path
					}),
					onFailure: (_) => Queue.offer(queue, {
						_tag: "Remove",
						path
					})
				}));
				return;
			case "change":
				Queue.offerUnsafe(queue, {
					_tag: "Update",
					path
				});
				return;
		}
	});
	watcher.on("error", (error) => {
		Queue.failCauseUnsafe(queue, Cause.fail(PlatformError.systemError({
			module: "FileSystem",
			_tag: "Unknown",
			method: "watch",
			pathOrDescriptor: path,
			cause: error
		})));
	});
	watcher.on("close", () => {
		Queue.endUnsafe(queue);
	});
	return watcher;
}), (watcher) => Effect.sync(() => watcher.close())).pipe(Effect.tap(() => Effect.sync(() => options?.onReady?.()))));
const watch = (backend, path, options) => Effect.all([stat(path), Effect.serviceOption(FileSystem.WatchBackend)]).pipe(Effect.map(([stat, callerBackend]) => Option$1.orElse(callerBackend, () => backend).pipe(Option$1.flatMap((_) => _.register(path, stat, options)), Option$1.getOrElse(() => watchNode(path, stat, options)))), Stream.unwrap);
const writeFile = (path, data, options) => Effect.callback((resume, signal) => {
	try {
		NFS.writeFile(path, data, {
			signal,
			flag: options?.flag,
			mode: options?.mode
		}, (err) => {
			if (err) resume(Effect.fail(handleErrnoException("FileSystem", "writeFile")(err, [path])));
			else resume(Effect.void);
		});
	} catch (err) {
		resume(Effect.fail(handleBadArgument("writeFile")(err)));
	}
});
const makeFileSystem = /*#__PURE__*/ Effect.map(/*#__PURE__*/ Effect.serviceOption(FileSystem.WatchBackend), (backend) => FileSystem.make({
	access,
	chmod,
	chown,
	copy,
	copyFile,
	glob,
	link,
	lstat,
	makeDirectory,
	makeTempDirectory,
	makeTempDirectoryScoped,
	makeTempFile,
	makeTempFileScoped,
	open,
	readDirectory,
	readFile,
	readLink,
	realPath,
	remove,
	rename,
	stat,
	symlink,
	truncate,
	utimes,
	watch(path, options) {
		return watch(backend, path, options);
	},
	writeFile
}));
// ../../node_modules/.pnpm/@effect+platform-node@4.0.6_effect@4.0.6/node_modules/@effect/platform-node/dist/NodeFileSystem.js
/**
* Node.js `FileSystem` layer for programs that perform real filesystem I/O.
*
* The exported layer satisfies the platform-independent `FileSystem` service
* with Node-backed operations for files, directories, metadata, permissions,
* links, temporary paths, and path watching. Effects still call the service from
* `effect/FileSystem`; this module only chooses the Node implementation.
*
* @stability unstable
* @since 4.0.0
*/
/**
* Provides the `FileSystem` service backed by Node filesystem APIs.
*
* @stability unstable
* @category layers
* @since 4.0.0
*/
const layer$7 = /* @__PURE__ */ Layer.effect(FileSystem.FileSystem)(makeFileSystem);
// ../../node_modules/.pnpm/@effect+platform-node-shared@4.0.6_effect@4.0.6/node_modules/@effect/platform-node-shared/dist/NodePath.js
/**
* Node-backed provider for Effect's `Path` service.
*
* This module turns Node's `node:path` and `node:url` APIs into `Path` layers.
* `layer` uses the host platform path implementation, while `layerPosix` and
* `layerWin32` provide fixed POSIX and Windows variants. All three layers also
* include helpers for converting between file paths and file URLs.
*
* @stability unstable
* @since 4.0.0
*/
const fileUrlOps = (windows) => ({
	fromFileUrl: (url) => Effect.try({
		try: () => NodeUrl.fileURLToPath(url, { windows }),
		catch: (cause) => new BadArgument({
			module: "Path",
			method: "fromFileUrl",
			cause
		})
	}),
	toFileUrl: (path) => Effect.try({
		try: () => NodeUrl.pathToFileURL(path, { windows }),
		catch: (cause) => new BadArgument({
			module: "Path",
			method: "toFileUrl",
			cause
		})
	})
});
({
	...Path$1.posix,
	.../*#__PURE__*/ fileUrlOps(false)
});
({
	...Path$1.win32,
	.../*#__PURE__*/ fileUrlOps(true)
});
// ../../node_modules/.pnpm/@effect+platform-node@4.0.6_effect@4.0.6/node_modules/@effect/platform-node/dist/NodePath.js
/**
* Node.js layers for Effect's `Path` service.
*
* This module provides the default, POSIX, and Windows variants of the
* platform-independent `Path` service by reusing the shared Node path
* implementation. The provided path services include Node file URL conversion
* behavior.
*
* @stability unstable
* @since 4.0.0
*/
/**
* Provides the default Node `Path` service using the platform's `node:path`
* implementation.
*
* @stability unstable
* @category layers
* @since 4.0.0
*/
const layer$5 = /* @__PURE__ */ Layer.succeed(Path)(/*#__PURE__*/ make({
	...Path$1,
	.../*#__PURE__*/ fileUrlOps(void 0)
}));
// ../../node_modules/.pnpm/@effect+platform-node@4.0.6_effect@4.0.6/node_modules/@effect/platform-node/dist/NodeStdio.js
/**
* Node.js `Stdio` layer for the current process.
*
* The exported layer reuses the shared Node stdio implementation. It satisfies
* the platform-independent `Stdio` service by reading command-line arguments
* from `process.argv`, consuming input from `process.stdin`, and writing output
* streams to `process.stdout` and `process.stderr`.
*
* @stability unstable
* @since 4.0.0
*/
/**
* Provides the `Stdio` service backed by the current process arguments,
* stdin, stdout, and stderr streams.
*
* @stability unstable
* @category layers
* @since 4.0.0
*/
const layer$3 = /* @__PURE__ */ Layer.succeed(Stdio.Stdio, /*#__PURE__*/ Stdio.make({
	args: /*#__PURE__*/ Effect.sync(() => process.argv.slice(2)),
	stdinIsTerminal: /*#__PURE__*/ Effect.sync(() => process.stdin.isTTY === true),
	stdoutIsTerminal: /*#__PURE__*/ Effect.sync(() => process.stdout.isTTY === true),
	stderrIsTerminal: /*#__PURE__*/ Effect.sync(() => process.stderr.isTTY === true),
	stdout: (options) => fromWritable({
		evaluate: () => process.stdout,
		onError: (cause) => systemError({
			module: "Stdio",
			method: "stdout",
			_tag: "Unknown",
			cause
		}),
		endOnDone: options?.endOnDone ?? false
	}),
	stderr: (options) => fromWritable({
		evaluate: () => process.stderr,
		onError: (cause) => systemError({
			module: "Stdio",
			method: "stderr",
			_tag: "Unknown",
			cause
		}),
		endOnDone: options?.endOnDone ?? false
	}),
	stdin: /*#__PURE__*/ fromReadable({
		evaluate: () => process.stdin,
		onError: (cause) => systemError({
			module: "Stdio",
			method: "stdin",
			_tag: "Unknown",
			cause
		}),
		closeOnDone: false
	})
}));
// ../../node_modules/.pnpm/@effect+platform-node-shared@4.0.6_effect@4.0.6/node_modules/@effect/platform-node-shared/dist/NodeTerminal.js
/**
* Creates a scoped process-backed `Terminal` using Node `readline`, enabling
* TTY raw mode while key input is read. By default, Esc, Ctrl+C, and Ctrl+D
* end key input. Supply `shouldQuit` to override these quit keys.
*
* @stability unstable
* @category constructors
* @since 4.0.0
*/
const make$2 = /*#__PURE__*/ Effect.fnUntraced(function* (shouldQuit = defaultShouldQuit) {
	const stdin = process.stdin;
	const stdout = process.stdout;
	const lines = yield* Queue.make();
	let inputEnded = stdin.readableEnded;
	let readlineActive = false;
	const onStdinEnd = () => {
		inputEnded = true;
		if (!readlineActive) Queue.endUnsafe(lines);
	};
	stdin.once("end", onStdinEnd);
	yield* Effect.addFinalizer(() => Effect.sync(() => stdin.off("end", onStdinEnd)));
	const rlRef = yield* RcRef.make({
		acquire: Effect.acquireRelease(Effect.sync(() => {
			const rl = readline.createInterface({
				input: stdin,
				escapeCodeTimeout: 50
			});
			const onLine = (line) => Queue.offerUnsafe(lines, line);
			const onClose = () => {
				readlineActive = false;
				Queue.endUnsafe(lines);
			};
			readlineActive = true;
			readline.emitKeypressEvents(stdin, rl);
			rl.on("line", onLine);
			rl.once("close", onClose);
			return {
				rl,
				onClose,
				onLine
			};
		}), ({ rl, onClose, onLine }) => Effect.sync(() => {
			readlineActive = false;
			rl.off("line", onLine);
			rl.off("close", onClose);
			rl.close();
			if (inputEnded) Queue.endUnsafe(lines);
		})),
		idleTimeToLive: "10 millis"
	});
	const setRawMode = (enabled) => Effect.sync(() => {
		if (stdin.isTTY) stdin.setRawMode(enabled);
	});
	const rawModeRef = yield* RcRef.make({
		acquire: Effect.acquireRelease(setRawMode(true), () => setRawMode(false)),
		idleTimeToLive: "10 millis"
	});
	const columns = Effect.sync(() => stdout.columns ?? 0);
	const rows = Effect.sync(() => stdout.rows ?? 0);
	const readInput = Effect.gen(function* () {
		const queue = yield* Queue.make();
		const handleKeypress = (s, k) => {
			const userInput = {
				input: Option$1.fromUndefinedOr(s),
				key: {
					name: k.name ?? "",
					ctrl: !!k.ctrl,
					meta: !!k.meta,
					shift: !!k.shift
				}
			};
			Queue.offerUnsafe(queue, userInput);
			if (shouldQuit(userInput)) Queue.endUnsafe(queue);
		};
		const keepAlive = setInterval(() => {}, 2147483647);
		const handleEnd = () => {
			clearInterval(keepAlive);
			Queue.endUnsafe(queue);
		};
		yield* Effect.addFinalizer(() => Effect.sync(() => {
			clearInterval(keepAlive);
			stdin.off("keypress", handleKeypress);
			stdin.off("end", handleEnd);
		}));
		stdin.on("keypress", handleKeypress);
		if (inputEnded) handleEnd();
		else {
			yield* RcRef.get(rlRef);
			yield* RcRef.get(rawModeRef);
			stdin.once("end", handleEnd);
		}
		return queue;
	});
	const readLine = Effect.suspend(() => Queue.poll(lines).pipe(Effect.flatMap(Option$1.match({
		onNone: () => Effect.scoped(Effect.andThen(RcRef.get(rlRef), Queue.take(lines))),
		onSome: Effect.succeed
	})), Effect.mapError(() => new Terminal.QuitError({}))));
	const display = (prompt) => Effect.uninterruptible(Effect.callback((resume) => {
		stdout.write(prompt, (err) => Predicate.isNullish(err) ? resume(Effect.void) : resume(Effect.fail(badArgument({
			module: "Terminal",
			method: "display",
			description: "Failed to write prompt to stdout",
			cause: err
		}))));
	}));
	return Terminal.make({
		columns,
		rows,
		readInput,
		readLine,
		display
	});
});
/**
* Provides the default process-backed `Terminal` service, ending key input on
* bare Esc, Ctrl+C, or Ctrl+D.
*
* @stability unstable
* @category layers
* @since 4.0.0
*/
const layer$2 = /*#__PURE__*/ Layer.effect(Terminal.Terminal, /*#__PURE__*/ make$2(defaultShouldQuit));
function defaultShouldQuit(input) {
	const key = input.key;
	return key.name === "escape" || key.ctrl && (key.name === "c" || key.name === "d");
}
// ../../node_modules/.pnpm/@effect+platform-node@4.0.6_effect@4.0.6/node_modules/@effect/platform-node/dist/NodeTerminal.js
/**
* Node.js implementation of the Effect `Terminal` service.
*
* This module reuses the shared Node terminal implementation. `make` creates a
* scoped process-backed `Terminal` service, and `layer` provides the default
* service with the standard quit behavior for key input.
*
* @stability unstable
* @since 4.0.0
*/
/**
* Provides the default process-backed `Terminal` service, ending key input on
* bare Esc, Ctrl+C, or Ctrl+D.
*
* @stability unstable
* @category layers
* @since 4.0.0
*/
const layer$1 = layer$2;
// ../../node_modules/.pnpm/@effect+platform-node@4.0.6_effect@4.0.6/node_modules/@effect/platform-node/dist/NodeServices.js
/**
* Provides the default Node implementations for child process spawning,
* filesystem, path, stdio, and terminal services.
*
* @stability unstable
* @category layers
* @since 4.0.0
*/
const layer = /*#__PURE__*/ Layer.provideMerge(layer$11, /*#__PURE__*/ Layer.mergeAll(layer$7, layer$9, layer$5, layer$3, layer$1));
// src/runtime.ts
const runtime = (configuration) => Layer.mergeAll(AgxClientLive, AgxConnectionLive(configuration.connection), SkillDeployerLive).pipe(Layer.provideMerge(Layer.merge(layer, Layer.succeed(Configuration, configuration))));
// src/index.ts
const register = Effect.fn("register")(function* (ctx) {
	const configuration = yield* Configuration;
	const path = yield* Path$2.Path;
	const reload = Effect.all([ctx.skill.reload(), ctx.mcp.reload()]).pipe(Effect.asVoid);
	const lease = yield* acquireLease(reload);
	yield* ctx.skill.transform((editor) => {
		for (const skill of lease.current().skills) editor.add(skill);
	});
	yield* ctx.mcp.transform((editor) => {
		const active = lease.current();
		for (const endpoint of active.endpoints) editor.set(endpoint.name, {
			type: "remote",
			url: endpoint.url.href,
			oauth: false,
			headers: { Authorization: `Bearer ${active.prepared.grant}` },
			...configuration.codemode === null ? {} : { codemode: configuration.codemode }
		});
	});
	const instructions = Effect.fn("instructions")(function* (event) {
		yield* lease.ensure;
		const text = lease.current().prepared.content.instructions;
		event.system = event.system.filter((part) => part.metadata?.["agx.instructions"] !== true);
		if (text !== "") event.system.push({
			type: "text",
			text,
			metadata: { "agx.instructions": true }
		});
	}, Effect.catchTags({
		ConnectionError: (error) => Effect.die(error),
		DaemonError: (error) => Effect.die(error),
		IntegrationError: (error) => Effect.die(error)
	}));
	yield* ctx.session.hook("context", instructions);
	yield* ctx.session.hook("compaction", instructions);
	yield* ctx.session.hook("generate", instructions);
	yield* ctx.session.hook("title", instructions);
	yield* ctx.shell.hook("create.before", (event) => lease.ensure.pipe(Effect.andThen(Effect.sync(() => {
		const active = lease.current();
		const names = [
			...Object.keys(active.environmentInputs),
			"AGX_MANAGEMENT_TOKEN",
			"AGX_LIFECYCLE_TOKEN",
			"AGX_LAUNCH_ID",
			"AGX_MCP_GRANT",
			"AGX_CONFIG"
		];
		const protectedNames = new Set(names.map((name) => path.sep === "\\" ? name.toUpperCase() : name));
		for (const name of names) event.env[name] = void 0;
		for (const name of Object.keys(event.env)) if (protectedNames.has(path.sep === "\\" ? name.toUpperCase() : name)) event.env[name] = void 0;
		event.env["AGX_LAUNCH_ID"] = active.prepared.launch.id;
		event.env["AGX_MCP_GRANT"] = active.prepared.grant;
		event.env["AGX_CONFIG"] = active.configuration;
	})), Effect.catchTags({
		ConnectionError: (error) => Effect.die(error),
		DaemonError: (error) => Effect.die(error),
		IntegrationError: (error) => Effect.die(error)
	})));
	yield* ctx.command.transform((editor) => editor.add({
		name: "agx-reload",
		description: "Reload this location's AGX configuration",
		execute: () => lease.refresh
	}));
	let warned = false;
	const warn = (error) => {
		if (warned) return Effect.void;
		warned = true;
		return Effect.logWarning(error.message);
	};
	yield* lease.ensure.pipe(Effect.tap(() => Effect.sync(() => {
		warned = false;
	})), Effect.catchTags({
		ConnectionError: warn,
		DaemonError: warn,
		IntegrationError: warn
	}), Effect.repeat(Schedule.spaced("1 second")), Effect.forkScoped);
	yield* Effect.logInfo("AGX resources ready", {
		skills: lease.current().skills.length,
		mcp: lease.current().prepared.mcp.length
	});
});
var src_default = define({
	id: "agx",
	effect: Effect.fn("agx.plugin")(function* (ctx) {
		const { harness, scopeDirectory, mcpDirectory, codemode, ...connection } = yield* Schema$1.decodeUnknownEffect(Options)(ctx.options).pipe(Effect.catchTag("SchemaError", (cause) => Effect.fail(new IntegrationError({
			operation: "plugin options",
			reason: "options",
			cause
		}))));
		const configuration = Configuration.of({
			connection,
			harness: harness ?? "opencode",
			scopeDirectory: scopeDirectory ?? ctx.location.directory,
			mcpDirectory: mcpDirectory ?? ctx.location.directory,
			codemode: codemode ?? null
		});
		const services = yield* Layer.build(runtime(configuration));
		yield* register(ctx).pipe(Effect.provide(services));
	}, Effect.catchTags({
		ConnectionError: (error) => Effect.die(error),
		DaemonError: (error) => Effect.die(error),
		IntegrationError: (error) => Effect.die(error)
	}))
});
export { src_default as default };
