/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import { z } from 'zod'

// Go/RE2 is authoritative for regex syntax; JavaScript rejects valid RE2 flags.
export const requestIdHeaderRuleSchema = z
  .string()
  .trim()
  .max(256, 'Header rule must not exceed 256 characters')
  .refine((value) => {
    if (!value) return true
    if (value.startsWith('re:')) return value.length > 3
    return /^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(value)
  }, 'Invalid request ID header rule')
