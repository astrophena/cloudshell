#!/usr/bin/env zsh

# © 2019 Ilya Mateyko. All rights reserved.
# Use of this source code is governed by the MIT
# license that can be found in the LICENSE.md file.

_cloudshell() {
  local -a opts
  opts=("${(@f)$(_CLI_ZSH_AUTOCOMPLETE_HACK=1 ${words[@]:0:#words[@]-1} --generate-bash-completion)}")

  _describe 'values' opts

  return
}

compdef _cloudshell cloudshell
