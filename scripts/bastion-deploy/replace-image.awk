# 只接受普通块式 services/service/image，不尝试实现完整 YAML 解析器。
function fail(message) { print "Compose 修改失败: " message > "/dev/stderr"; bad=1; exit 1 }
function keyname(text) {
  sub(/:.*/, "", text)
  if (text ~ /^"[^"\\]+"$/ || text ~ /^'[^']+'$/) text=substr(text,2,length(text)-2)
  return text
}
BEGIN { target=ENVIRON["AETHER_SERVICE"]; image=ENVIRON["AETHER_IMAGE"]; service_indent=-1; field_indent=-1 }
{
  original=$0
  if ($0 ~ /^[ ]*#/ || $0 ~ /^[ ]*$/) { print; next }
  if ($0 ~ /\t/) fail("不支持制表符")
  match($0,/[^ ]/); indent=RSTART-1
  text=substr($0,indent+1)
  if (indent==0) {
    in_target=0; in_services=0
    if (keyname(text)=="services") {
      if (++sections>1) fail("重复 services")
      if (text !~ /^services:[ ]*(#.*)?$/) fail("services 必须为普通块式结构")
      in_services=1
    }
  } else if (in_services) {
    if (service_indent<0) service_indent=indent
    if (indent<service_indent) fail("services 缩进不一致")
    if (indent==service_indent) {
      in_target=(keyname(text)==target)
      if (in_target) {
        if (++found>1) fail("重复目标服务")
        if (text !~ /:[ ]*(#.*)?$/) fail("目标服务不支持内联结构或锚点")
      }
    } else if (in_target) {
      if (field_indent<0) field_indent=indent
      if (indent<field_indent) fail("服务字段缩进不一致")
      if (indent==field_indent) {
        field=keyname(text)
        if (field=="<<") fail("目标服务不支持锚点继承")
        if (field=="image") {
          if (++images>1) fail("重复 image 字段")
          value=text; sub(/^[^:]+:[ ]*/,"",value)
          # 限定为普通镜像引用（可带引号），拒绝变量、别名和复杂 YAML。
          plain=value; sub(/[ ]+#.*$/, "", plain); sub(/[ ]+$/, "", plain)
          if (plain ~ /^".*"$/ || plain ~ /^'.*'$/) plain=substr(plain,2,length(plain)-2)
          if (plain !~ /^[a-zA-Z0-9][a-zA-Z0-9_.\/:@-]*$/) fail("image 不是支持的普通镜像字符串")
          comment=value
          if (match(comment, /[ ]+#/)) comment=substr(comment,RSTART); else comment=""
          print "旧镜像: " plain > "/dev/stderr"
          prefix=substr(original,1,index(original,":"))
          original=prefix " " image comment
        }
      }
    }
  }
  print original
}
END {
  if (bad) exit 1
  if (sections!=1 || found!=1 || images!=1) { print "Compose 修改失败: 未找到唯一的 services/目标服务/image" > "/dev/stderr"; exit 1 }
}
