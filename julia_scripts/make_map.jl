using CSV, DataFrames, Gadfly, StatsBase
using Cairo, Fontconfig

INDIR = "/home/valentin/Development/fuzzing_exps/1112/tiff_out2_long"
OUTDIR = "figures"

base_dir = split(INDIR, "/")[end]
println("Analyzing: $(base_dir)")

################################################################################
################################# 1. Make Map ##################################

function calcNorm(x1, x2, y1, y2)
    xd = x2 - x1
    yd = y2 - y1
    return sqrt(xd*xd + yd*yd)
end
function prepareData(df::DataFrame, xSym::Symbol, ySym::Symbol, sigX::Float64 ,sigY::Float64)
    seeds = filter(e->e[:kind]=="seed", df)
    otherDims = filter(e-> !(e in [:index, xSym, ySym]), names(seeds))
    deletecols!(seeds, otherDims)
    rename!(seeds, Dict(xSym => :x, ySym => :y))
    #
    centers = filter(e->e[:kind]=="center", df)
    deletecols!(centers, otherDims)
    rename!(centers, Dict(xSym => :xend, ySym => :yend))
    
    data = join(seeds, centers, on = :index)
    data[:norm] = map((x1, x2, y1, y2) -> calcNorm(x1, x2, y1, y2),
        data[:x], data[:xend], data[:y], data[:yend])
    return data
end
function doPlotFlow(data::DataFrame, xl::String, yl::String)
    l = layer(data,
        x = :x, y = :y, xend = :xend, yend = :yend, color = :norm,
        Geom.segment(arrow=true), Geom.point,
    )

    xmin = min(min(data[:xend]...), min(data[:x]...))
    xmax = max(max(data[:xend]...), max(data[:x]...))
    ymin = min(min(data[:yend]...), min(data[:y]...))
    ymax = max(max(data[:yend]...), max(data[:y]...))

    coord = Coord.cartesian(xmin = xmin, xmax = xmax, ymin = ymin, ymax = ymax)
    xsc  = Scale.x_continuous(minvalue = xmin, maxvalue = xmax)
    ysc  = Scale.y_continuous(minvalue = ymin, maxvalue = ymax)
    colsc = Scale.color_continuous(minvalue = min(data[:norm]...), maxvalue = max(data[:norm]...))
    return plot(l, xsc, ysc, colsc, coord, Guide.xlabel(xl), Guide.ylabel(yl))
end
function plotFlow(df::DataFrame, xSym::Symbol, ySym::Symbol, varX::Float64 ,varY::Float64)
    data = prepareData(df, xSym, ySym, sqrt(varX), sqrt(varY))
    return doPlotFlow(data, string(xSym), string(ySym))
end

df = CSV.read("$(INDIR)/coords.csv")
vars = filter(e->e[:kind]=="variance", df)[1, 3:end]

pcs = names(df)[3:end]
pcN = 6 #length(pcs)
plotN = convert(Int, pcN * (pcN - 1) / 2)
println("#Plot: $(plotN)")
plots = Array{Plot}(undef, plotN)

plotI = 1
for i in 1:pcN
    for j in (i+1):pcN
        global plotI
        plots[plotI] = plotFlow(df, pcs[i], pcs[j], vars[i], vars[j])
        plotI += 1
    end
end

set_default_plot_size(20cm, plotN*20cm)
p = vstack(plots);

p |> PDF("$(OUTDIR)/$(base_dir)_multidim_map.pdf")

################################################################################
############################ 2. Divergence Analysis ############################

function lincor(x, y)
    n = length(x)
    prod = sum(x .* y)
    ux, sx = mean_and_std(x)
    uy, sy = mean_and_std(y)
    cor = prod / n - ux * uy
    cor = cor / (sx * sy)
    return cor
end
function getKind(df::DataFrame, k::String)
    tmp = filter(e->e[:kind]==k, df)
    deletecols!(tmp, :kind)
    rename!(tmp, Dict(:value => Symbol(k)))
    sort!(tmp, Symbol(k))
    return tmp
end

df = CSV.read("$(INDIR)/distances.csv")
df = filter(e -> !isinf(e[:value]) , df)

df_divs = join(getKind(df, "divergence"), getKind(df, "mle_divergence"),
                   on = [:index1, :index2])
df_divs = join(df_divs, getKind(df, "hist_divergence"), on = [:index1, :index2])
#
s_cor = corspearman(df_divs[:divergence], df_divs[:mle_divergence])
p_cor = lincor(df_divs[:divergence], df_divs[:mle_divergence])
println("[Normal-MLE divergences correlation] Spearman: $(s_cor)\tPearson: $(p_cor) .")
#
s_cor = corspearman(df_divs[:divergence], df_divs[:hist_divergence])
p_cor = lincor(df_divs[:divergence], df_divs[:hist_divergence])
println("[Normal-Hist divergences correlation]\tSpearman: $(s_cor)\tPearson: $(p_cor) .")
#
s_cor = corspearman(df_divs[:mle_divergence], df_divs[:hist_divergence])
p_cor = lincor(df_divs[:mle_divergence], df_divs[:hist_divergence])
println("[MLE-Hist divergences correlation]\tSpearman: $(s_cor)\tPearson: $(p_cor) .")
println("Size of df_divs: $(size(df_divs))")


symbolsStr = ["c2c_full_eucli", "c2c_proj_eucli", "c2c_maha",
    "s2s_full_eucli", "s2s_proj_eucli", "s2s_maha"]
println("\nDistance-MLE_Div correlations:")
for str in symbolsStr
    sym = Symbol(str)
    div_df = join(getKind(df, str), getKind(df, "mle_divergence"), on = [:index1, :index2])
    div_df[:id] = 1:(size(div_df)[1])
    s_cor = corspearman(div_df[sym], div_df[:mle_divergence])
    p_cor = lincor(div_df[sym], div_df[:mle_divergence])
    println("[$(str)]\tspearman: $(s_cor)\tp_cor: $(p_cor) .")
end
#
println("\nDistance-Normal_Div correlations:")
for str in symbolsStr
    sym = Symbol(str)
    div_df = join(getKind(df, str), getKind(df, "divergence"), on = [:index1, :index2])
    div_df[:id] = 1:(size(div_df)[1])
    s_cor = corspearman(div_df[sym], div_df[:divergence])
    p_cor = lincor(div_df[sym], div_df[:divergence])
    println("[$(str)]\tspearman: $(s_cor)\tp_cor: $(p_cor) .")
end
#
println("\nDistance-Hist_Div correlations:")
for str in symbolsStr
    sym = Symbol(str)
    div_df = join(getKind(df, str), getKind(df, "hist_divergence"), on = [:index1, :index2])
    div_df[:id] = 1:(size(div_df)[1])
    s_cor = corspearman(div_df[sym], div_df[:hist_divergence])
    p_cor = lincor(div_df[sym], div_df[:hist_divergence])
    println("[$(str)]\tspearman: $(s_cor)\tp_cor: $(p_cor) .")
end

eucli_df = join(getKind(df, "c2c_full_eucli"), getKind(df, "c2c_proj_eucli"), on = [:index1, :index2])
pca_loss = mean(eucli_df[:c2c_proj_eucli] ./ eucli_df[:c2c_full_eucli])
println("\nPCA loss: $(100*(1-pca_loss))")
