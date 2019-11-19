using CSV, DataFrames, Gadfly, StatsBase

df = CSV.read("distances_tiff.csv")

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

symbolsStr = ["c2c_full_eucli", "c2c_proj_eucli", "c2c_maha",
              "s2s_full_eucli", "s2s_proj_eucli", "s2s_maha"]
println("\nDistance-Hist_Div correlations:")
for str in symbolsStr
    sym = Symbol(str)
    div_df = join(getKind(df, str), getKind(df, "hist_divergence"), on = [:index1, :index2])
    div_df[:id] = 1:(size(div_df)[1])
    s_cor = corspearman(div_df[sym], div_df[:hist_divergence])
    p_cor = lincor(div_df[sym], div_df[:hist_divergence])
    println("[$(str)]\tspearman: $(s_cor)\tp_cor: $(p_cor) .")
end

df_divs = join(getKind(df, "divergence"), getKind(df, "mle_divergence"),
                   on = [:index1, :index2])
println(size(df_divs))
df_divs = join(df_divs, getKind(df, "hist_divergence"), on = [:index1, :index2])
println(size(df_divs))

s_cor = corspearman(df_divs[:divergence], df_divs[:hist_divergence])
p_cor = lincor(df_divs[:divergence], df_divs[:hist_divergence])
println("[Normal-Hist divergences correlation]\tSpearman: $(s_cor)\tPearson: $(p_cor) .")

s_cor = corspearman(df_divs[:mle_divergence], df_divs[:hist_divergence])
p_cor = lincor(df_divs[:mle_divergence], df_divs[:hist_divergence])
println("[MLE-Hist divergences correlation]\tSpearman: $(s_cor)\tPearson: $(p_cor) .")

s_cor = corspearman(df_divs[:mle_divergence], df_divs[:divergence])
p_cor = lincor(df_divs[:mle_divergence], df_divs[:divergence])
println("[MLE-Normal divergences correlation]\tSpearman: $(s_cor)\tPearson: $(p_cor) .")
