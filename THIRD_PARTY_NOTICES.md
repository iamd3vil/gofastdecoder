# Third-Party Notices

This project (`gofastdecoder`, MIT-licensed) incorporates test material derived
from third-party software. The relevant licenses and attributions are below.

## mFAST

- **Project:** mFAST — a high-performance C++ FAST encoder/decoder
- **Source:** https://github.com/objectcomputing/mFAST
- **Copyright:** Copyright (c) 2016, Object Computing, Inc. All rights reserved.
- **License:** BSD 3-Clause "New" or "Revised" License
- **Full license text:** [`testdata/mfast/LICENSE.mfast`](testdata/mfast/LICENSE.mfast)

### What is used

- **Templates** under `testdata/mfast/templates/` — copied verbatim from
  mFAST's `tests/` directory (`simple*.xml`, `test*.xml`, `scp.xml`).
- **Upstream test sources** under `testdata/mfast/upstream/` — copied verbatim
  for audit/reference. These retain mFAST's original file headers (which predate
  the 2016 relicensing and still read "LGPLv3"; the repository's `licence.txt`
  and `ReadMe.md` establish the authoritative license as BSD-3-Clause as of
  2016-03-13).
- **Transcribed vectors** under `testdata/vectors/` — the encoded byte sequences
  and expected decoded values were transcribed by hand from the mFAST test
  sources into a language-neutral JSON corpus. Each file's `source` field cites
  the originating mFAST test file.

Use of this material complies with the BSD 3-Clause license: the copyright
notice and license text are retained above and in `testdata/mfast/LICENSE.mfast`.
