# Use this resource when programs in the same `for_each` block need to
# reference each other as parent — Terraform's self-reference cycle
# prevents writing `parent` directly on `st-zentao_program`.

resource "st-zentao_program" "parent" {
  name  = "Smart Home"
  begin = "2026-01-01"
  end   = "2026-12-31"
}

resource "st-zentao_program" "child" {
  name  = "Smart Home — Voice"
  begin = "2026-01-01"
  end   = "2026-12-31"
}

resource "st-zentao_program_parent_attachment" "child_under_parent" {
  program = st-zentao_program.child.id
  parent  = st-zentao_program.parent.id
}
