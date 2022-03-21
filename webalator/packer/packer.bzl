def _webalator_content_pack_impl(ctx):
    output_file = ctx.actions.declare_file(ctx.label.name + ".webalator.zip")

    args = ctx.actions.args()
    args.add("--output", output_file)
    args.add_all(ctx.files.static_files, format_each='--static_file=%s')
    args.add("--static_file_trim_prefix", ctx.attr.static_file_trim_prefix)
    args.add_all(ctx.files.template_files, format_each='--template_file=%s')
    args.add("--template_file_trim_prefix", ctx.attr.template_file_trim_prefix)

    ctx.actions.run(
        outputs = [output_file],
        inputs = depset(ctx.files.static_files + ctx.files.template_files),
        executable = ctx.executable._packer,
        arguments = [args],
        progress_message = "Packing {}".format(output_file.short_path),
    )

    return [
        DefaultInfo(files = depset([output_file])),
    ]

# webalator_content_pack declares a content pack that can be loaded and served
# by webalator.
webalator_content_pack = rule(
    implementation = _webalator_content_pack_impl,
    attrs = {
        "static_files": attr.label_list(allow_files = True),
        "static_file_trim_prefix": attr.string(),
        "template_files": attr.label_list(allow_files = True),
        "template_file_trim_prefix": attr.string(),
        "_packer": attr.label(
            default = Label("//webalator/packer:packer"),
            allow_single_file = True,
            executable = True,
            cfg = "exec",
        ),
    },
)
