# 👀 Examples

## Section: Strain your brain!

### ▶ Be careful with chained operations

```py
>>> (False == False) in [False] # makes sense
False
>>> False == (False in [False]) # makes sense
False
>>> False == False in [False] # now what?
True

>>> True is False == False
False
>>> False is False is False
True
```

#### 💡 Explanation:

As per https://docs.python.org/3/reference/expressions.html#comparisons

> Formally, if a, b, c, ..., y, z are expressions and op1, op2, ..., opN are comparison operators,
  then a op1 b op2 c ... y opN z is equivalent to a op1 b and b op2 c and ... y opN z,
  except that each expression is evaluated at most once.

While such behavior might seem silly to you in the above examples,
it's fantastic with stuff like `a == b == c` and `0 <= x <= 100`.

### ▶ Hash brownies

```py
some_dict = {}
some_dict[5.5] = "JavaScript"
some_dict[5.0] = "Ruby"
some_dict[5] = "Python"
```

**Output:**

```py
>>> some_dict[5.5]
"JavaScript"
>>> some_dict[5.0] # "Python" destroyed the existence of "Ruby"?
"Python"
>>> some_dict[5]
"Python"
```

#### 💡 Explanation

- Uniqueness of keys in a Python dictionary is by _equivalence_, not identity. So even though `5`, `5.0`, and `5 + 0j` are distinct objects of different types, since they're equal, they can't both be in the same `dict` (or `set`).
- So how can we update the key to `5` (instead of `5.0`)? We can't actually do this update in place, but what we can do is first delete the key (`del some_dict[5.0]`), and then set it (`some_dict[5]`) to get the integer `5` as the key instead of floating `5.0`, though this should be needed in rare cases.

### ▶ Deep down, we're all the same.

```py
class WTF:
  pass
```

**Output:**

```py
>>> WTF() == WTF() # two different instances can't be equal
False
>>> WTF() is WTF() # identities are also different
False
>>> hash(WTF()) == hash(WTF()) # hashes _should_ be different as well
True
>>> id(WTF()) == id(WTF())
True
```

#### 💡 Explanation:

- When `id` was called, Python created a `WTF` class object and passed it to the `id` function. The `id` function takes its `id` (its memory location), and throws away the object. The object is destroyed.
- So, the object's id is unique only for the lifetime of the object. After the object is destroyed, or before it is created, something else can have the same id.

### ▶ Malformed missing explanation

```py
>>> 1 + 1
2
```

# Contributing

Footer to skip.
