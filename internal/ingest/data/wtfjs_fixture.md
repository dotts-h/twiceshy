# 👀 Examples

## `[]` is equal `![]`

Array is equal not array:

```js
[] == ![]; // -> true
```

### 💡 Explanation:

The abstract equality operator converts both sides to numbers to compare them, and both sides become the number `0` for different reasons. Arrays are truthy, so on the right, the opposite of a truthy value is `false`, which is then coerced to `0`. On the left, however, an empty array is coerced to a number without becoming a boolean first, and empty arrays are coerced to `0`, despite being truthy.

## baNaNa

```js
"b" + "a" + +"a" + "a"; // -> 'baNaNa'
```

### 💡 Explanation:

The expression is evaluated as `'foo' + (+'bar')`, which converts `'bar'` to not a number.

## `NaN` is not a `NaN`

```js
NaN === NaN; // -> false
```

### 💡 Explanation:

The specification strictly defines the logic behind this behavior:

> 1. If `Type(x)` is different from `Type(y)`, return **false**.
> 2. If `Type(x)` is Number, then
>    1. If `x` is **NaN**, return **false**.
>    2. If `y` is **NaN**, return **false**.

NaN === NaN being false is apparently due to historical reasons so it would probably be better to accept it as it is.

## Adding arrays

```js
[] + []; // -> ''
```

### 💡 Explanation:

The addition operator coerces both operands to strings. Empty arrays stringify to empty strings, so `[] + []` yields `''`.

## Malformed missing explanation

```js
1 + 1; // -> 2
```

# 📚 Other resources

Some footer that must be skipped.
